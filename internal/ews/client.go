package ews

import (
	"bytes"
	"crypto/tls"
	"encoding/xml"
	"fmt"
	"io"
	"log"
	"net/http"
	"regexp"
	"strings"
	"time"

	"ews-meeting-reminders/internal/joinurl"

	"github.com/Azure/go-ntlmssp"
)

const getItemBatchSize = 50

type Meeting struct {
	ID       string
	Subject  string
	Start    time.Time
	Location string
	JoinURL  string
	Response string
}

type Client struct {
	endpoint   string
	email      string
	httpClient *http.Client
	joinHosts  []string
	include    map[string]struct{}
}

func New(endpoint, email, username, password, auth string, verifySSL bool, joinHosts []string, include map[string]struct{}) *Client {
	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: !verifySSL, //nolint:gosec
		},
	}

	var rt http.RoundTripper
	switch strings.ToLower(auth) {
	case "basic":
		rt = &basicAuthRoundTripper{username: username, password: password, next: transport}
	default: // ntlm
		rt = &ntlmAuthRoundTripper{
			username: username,
			password: password,
			next:     ntlmssp.Negotiator{RoundTripper: transport},
		}
	}
	// http.Client.CloseIdleConnections only reaches Transport if it implements
	// CloseIdleConnections; auth wrappers otherwise swallow it (no-op).
	rt = &idleCloseRoundTripper{RoundTripper: rt, idle: transport}

	return &Client{
		endpoint:   endpoint,
		email:      email,
		httpClient: &http.Client{Transport: rt, Timeout: 60 * time.Second},
		joinHosts:  joinHosts,
		include:    include,
	}
}

type idleCloseRoundTripper struct {
	http.RoundTripper
	idle interface{ CloseIdleConnections() }
}

func (r *idleCloseRoundTripper) CloseIdleConnections() {
	r.idle.CloseIdleConnections()
}

type basicAuthRoundTripper struct {
	username, password string
	next               http.RoundTripper
}

func (b *basicAuthRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	req2 := req.Clone(req.Context())
	req2.SetBasicAuth(b.username, b.password)
	return b.next.RoundTrip(req2)
}

type ntlmAuthRoundTripper struct {
	username, password string
	next               http.RoundTripper
}

func (n *ntlmAuthRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	req2 := req.Clone(req.Context())
	req2.SetBasicAuth(n.username, n.password)
	return n.next.RoundTrip(req2)
}

type itemRef struct {
	ID, ChangeKey string
}

func (c *Client) CalendarView(start, end time.Time) ([]Meeting, error) {
	raw, err := c.doSOAP("FindItem", buildFindItem(c.email, start, end), true)
	if err != nil {
		return nil, fmt.Errorf("FindItem: %w", err)
	}
	meetings, refs, err := parseFindItem(raw, c.joinHosts, c.include)
	if err != nil {
		return nil, err
	}
	need := make([]itemRef, 0, len(refs))
	for i, m := range meetings {
		if m.JoinURL != "" {
			continue
		}
		ref, ok := refs[i]
		if !ok || ref.ID == "" {
			continue
		}
		need = append(need, ref)
	}
	if len(need) == 0 {
		return meetings, nil
	}
	// Fresh TCP/TLS+NTLM handshake: reused keep-alive conns often EOF on the next EWS call.
	c.httpClient.CloseIdleConnections()
	bodies, err := c.fetchBodies(need)
	if err != nil {
		return nil, fmt.Errorf("GetItem: %w", err)
	}
	for i := range meetings {
		if meetings[i].JoinURL != "" {
			continue
		}
		body, ok := bodies[meetings[i].ID]
		if !ok || body == "" {
			continue
		}
		meetings[i].JoinURL = joinurl.Extract(meetings[i].Location, body, c.joinHosts)
	}
	return meetings, nil
}

func (c *Client) fetchBodies(refs []itemRef) (map[string]string, error) {
	out := make(map[string]string, len(refs))
	for i := 0; i < len(refs); i += getItemBatchSize {
		if i > 0 {
			c.httpClient.CloseIdleConnections()
		}
		end := i + getItemBatchSize
		if end > len(refs) {
			end = len(refs)
		}
		raw, err := c.doSOAP("GetItem", buildGetItem(refs[i:end]), false)
		if err != nil {
			return nil, err
		}
		part, err := parseGetItemBodies(raw)
		if err != nil {
			return nil, err
		}
		for id, body := range part {
			out[id] = body
		}
	}
	return out, nil
}

func (c *Client) doSOAP(action, body string, failOnErrorClass bool) ([]byte, error) {
	req, err := http.NewRequest(http.MethodPost, c.endpoint, strings.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "text/xml; charset=utf-8")
	req.Header.Set("SOAPAction", `"http://schemas.microsoft.com/exchange/services/2006/messages/`+action+`"`)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 300 {
		snippet := string(data)
		if len(snippet) > 500 {
			snippet = snippet[:500]
		}
		return nil, fmt.Errorf("ews HTTP %s: %s", resp.Status, snippet)
	}
	if failOnErrorClass && (bytes.Contains(data, []byte("ResponseClass=\"Error\"")) || bytes.Contains(data, []byte("ResponseClass='Error'"))) {
		snippet := string(data)
		if len(snippet) > 800 {
			snippet = snippet[:800]
		}
		return nil, fmt.Errorf("ews SOAP error: %s", snippet)
	}
	return data, nil
}

func buildFindItem(email string, start, end time.Time) string {
	mailbox := ""
	if email != "" {
		mailbox = fmt.Sprintf(`
          <t:Mailbox>
            <t:EmailAddress>%s</t:EmailAddress>
          </t:Mailbox>`, xmlEscape(email))
	}
	// FindItem does not return item:Body (EWS limitation). Body is loaded via GetItem.
	return fmt.Sprintf(`<?xml version="1.0" encoding="utf-8"?>
<soap:Envelope xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"
  xmlns:m="http://schemas.microsoft.com/exchange/services/2006/messages"
  xmlns:t="http://schemas.microsoft.com/exchange/services/2006/types"
  xmlns:soap="http://schemas.xmlsoap.org/soap/envelope/">
  <soap:Header>
    <t:RequestServerVersion Version="Exchange2013_SP1"/>
  </soap:Header>
  <soap:Body>
    <m:FindItem Traversal="Shallow">
      <m:ItemShape>
        <t:BaseShape>IdOnly</t:BaseShape>
        <t:AdditionalProperties>
          <t:FieldURI FieldURI="item:Subject"/>
          <t:FieldURI FieldURI="calendar:Start"/>
          <t:FieldURI FieldURI="calendar:Location"/>
          <t:FieldURI FieldURI="calendar:IsCancelled"/>
          <t:FieldURI FieldURI="calendar:MyResponseType"/>
        </t:AdditionalProperties>
      </m:ItemShape>
      <m:CalendarView MaxEntriesReturned="200" StartDate="%s" EndDate="%s"/>
      <m:ParentFolderIds>
        <t:DistinguishedFolderId Id="calendar">%s
        </t:DistinguishedFolderId>
      </m:ParentFolderIds>
    </m:FindItem>
  </soap:Body>
</soap:Envelope>`, start.UTC().Format(time.RFC3339), end.UTC().Format(time.RFC3339), mailbox)
}

func buildGetItem(refs []itemRef) string {
	var ids strings.Builder
	for _, r := range refs {
		if r.ChangeKey != "" {
			fmt.Fprintf(&ids, "\n        <t:ItemId Id=\"%s\" ChangeKey=\"%s\"/>", xmlEscape(r.ID), xmlEscape(r.ChangeKey))
		} else {
			fmt.Fprintf(&ids, "\n        <t:ItemId Id=\"%s\"/>", xmlEscape(r.ID))
		}
	}
	return fmt.Sprintf(`<?xml version="1.0" encoding="utf-8"?>
<soap:Envelope xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"
  xmlns:m="http://schemas.microsoft.com/exchange/services/2006/messages"
  xmlns:t="http://schemas.microsoft.com/exchange/services/2006/types"
  xmlns:soap="http://schemas.xmlsoap.org/soap/envelope/">
  <soap:Header>
    <t:RequestServerVersion Version="Exchange2013_SP1"/>
  </soap:Header>
  <soap:Body>
    <m:GetItem>
      <m:ItemShape>
        <t:BaseShape>IdOnly</t:BaseShape>
        <t:BodyType>HTML</t:BodyType>
        <t:AdditionalProperties>
          <t:FieldURI FieldURI="item:Body"/>
        </t:AdditionalProperties>
      </m:ItemShape>
      <m:ItemIds>%s
      </m:ItemIds>
    </m:GetItem>
  </soap:Body>
</soap:Envelope>`, ids.String())
}

func xmlEscape(s string) string {
	var b strings.Builder
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}

type findItemDoc struct {
	XMLName xml.Name `xml:"Envelope"`
	Body    struct {
		FindItemResponse struct {
			ResponseMessages struct {
				FindItemResponseMessage struct {
					ResponseClass string `xml:"ResponseClass,attr"`
					MessageText   string `xml:"MessageText"`
					RootFolder    struct {
						Items struct {
							CalendarItems []calendarItem `xml:"CalendarItem"`
						} `xml:"Items"`
					} `xml:"RootFolder"`
				} `xml:"FindItemResponseMessage"`
			} `xml:"ResponseMessages"`
		} `xml:"FindItemResponse"`
	} `xml:"Body"`
}

type getItemDoc struct {
	XMLName xml.Name `xml:"Envelope"`
	Body    struct {
		GetItemResponse struct {
			ResponseMessages struct {
				GetItemResponseMessage []struct {
					ResponseClass string `xml:"ResponseClass,attr"`
					MessageText   string `xml:"MessageText"`
					Items         struct {
						CalendarItems []calendarItem `xml:"CalendarItem"`
						Items         []calendarItem `xml:"Item"`
					} `xml:"Items"`
				} `xml:"GetItemResponseMessage"`
			} `xml:"ResponseMessages"`
		} `xml:"GetItemResponse"`
	} `xml:"Body"`
}

type calendarItem struct {
	ItemID struct {
		ID        string `xml:"Id,attr"`
		ChangeKey string `xml:"ChangeKey,attr"`
	} `xml:"ItemId"`
	Subject        string `xml:"Subject"`
	Body           string `xml:"Body"`
	Start          string `xml:"Start"`
	Location       string `xml:"Location"`
	IsCancelled    bool   `xml:"IsCancelled"`
	MyResponseType string `xml:"MyResponseType"`
}

var (
	reXMLNS   = regexp.MustCompile(`\sxmlns(:\w+)?="[^"]*"`)
	reTagPref = regexp.MustCompile(`</?[A-Za-z0-9]+:`)
)

func stripXMLNS(data []byte) []byte {
	s := string(data)
	s = reXMLNS.ReplaceAllString(s, "")
	s = reTagPref.ReplaceAllStringFunc(s, func(m string) string {
		if strings.HasPrefix(m, "</") {
			return "</"
		}
		return "<"
	})
	return []byte(s)
}

// parseFindItem returns meetings and ItemId refs keyed by meeting index (for GetItem).
func parseFindItem(data []byte, joinHosts []string, include map[string]struct{}) ([]Meeting, map[int]itemRef, error) {
	cleaned := stripXMLNS(data)
	var doc findItemDoc
	if err := xml.Unmarshal(cleaned, &doc); err != nil {
		return nil, nil, fmt.Errorf("parse FindItem: %w", err)
	}
	msg := doc.Body.FindItemResponse.ResponseMessages.FindItemResponseMessage
	if msg.ResponseClass == "Error" {
		return nil, nil, fmt.Errorf("FindItem: %s", msg.MessageText)
	}
	var out []Meeting
	refs := make(map[int]itemRef)
	for _, it := range msg.RootFolder.Items.CalendarItems {
		if it.IsCancelled {
			continue
		}
		resp := normalizeResponse(it.MyResponseType)
		if len(include) > 0 {
			if _, ok := include[resp]; !ok {
				continue
			}
		}
		start, err := parseEWSTime(it.Start)
		if err != nil {
			log.Printf("skip item %s: bad start %q", it.ItemID.ID, it.Start)
			continue
		}
		subj := strings.TrimSpace(it.Subject)
		if subj == "" {
			subj = "(без темы)"
		}
		loc := strings.TrimSpace(it.Location)
		join := joinurl.Extract(loc, it.Body, joinHosts)
		id := it.ItemID.ID
		if id == "" {
			id = subj + "|" + start.Format(time.RFC3339)
		}
		idx := len(out)
		out = append(out, Meeting{
			ID:       id,
			Subject:  subj,
			Start:    start.In(time.Local),
			Location: loc,
			JoinURL:  join,
			Response: resp,
		})
		if it.ItemID.ID != "" {
			refs[idx] = itemRef{ID: it.ItemID.ID, ChangeKey: it.ItemID.ChangeKey}
		}
	}
	return out, refs, nil
}

func parseGetItemBodies(data []byte) (map[string]string, error) {
	cleaned := stripXMLNS(data)
	var doc getItemDoc
	if err := xml.Unmarshal(cleaned, &doc); err != nil {
		return nil, fmt.Errorf("parse GetItem: %w", err)
	}
	out := make(map[string]string)
	for _, msg := range doc.Body.GetItemResponse.ResponseMessages.GetItemResponseMessage {
		if msg.ResponseClass == "Error" {
			log.Printf("GetItem item error: %s", msg.MessageText)
			continue
		}
		items := append(msg.Items.CalendarItems, msg.Items.Items...)
		for _, it := range items {
			id := it.ItemID.ID
			if id == "" {
				continue
			}
			out[id] = it.Body
		}
	}
	return out, nil
}

func parseEWSTime(s string) (time.Time, error) {
	formats := []string{
		time.RFC3339,
		time.RFC3339Nano,
		"2006-01-02T15:04:05Z07:00",
		"2006-01-02T15:04:05",
	}
	var last error
	for _, f := range formats {
		t, err := time.Parse(f, s)
		if err == nil {
			if t.Location() == time.UTC && !strings.ContainsAny(s, "Z+-") {
				// bare local wall time from some servers — treat as local
				t = time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), t.Minute(), t.Second(), t.Nanosecond(), time.Local)
			}
			return t, nil
		}
		last = err
	}
	return time.Time{}, last
}

func normalizeResponse(v string) string {
	if v == "" {
		return "Unknown"
	}
	if i := strings.LastIndex(v, "."); i >= 0 {
		return v[i+1:]
	}
	return v
}
