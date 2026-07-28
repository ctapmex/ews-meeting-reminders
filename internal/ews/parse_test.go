package ews

import (
	"strings"
	"testing"
)

func TestParseFindItem(t *testing.T) {
	xml := []byte(`<?xml version="1.0"?>
<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/">
 <s:Body>
  <m:FindItemResponse xmlns:m="http://schemas.microsoft.com/exchange/services/2006/messages"
                      xmlns:t="http://schemas.microsoft.com/exchange/services/2006/types">
   <m:ResponseMessages>
    <m:FindItemResponseMessage ResponseClass="Success">
     <m:ResponseCode>NoError</m:ResponseCode>
     <m:RootFolder>
      <t:Items>
       <t:CalendarItem>
        <t:ItemId Id="ABC123" ChangeKey="X"/>
        <t:Subject>Standup</t:Subject>
        <t:Body>see https://x.ktalk.ru/bss</t:Body>
        <t:Start>2026-07-22T12:05:00Z</t:Start>
        <t:Location>https://trueconf.x.com/c/1</t:Location>
        <t:IsCancelled>false</t:IsCancelled>
        <t:MyResponseType>Accept</t:MyResponseType>
       </t:CalendarItem>
      </t:Items>
     </m:RootFolder>
    </m:FindItemResponseMessage>
   </m:ResponseMessages>
  </m:FindItemResponse>
 </s:Body>
</s:Envelope>`)
	hosts := []string{"*.ktalk.ru", "trueconf.x.com"}
	include := map[string]struct{}{"Accept": {}}
	got, refs, err := parseFindItem(xml, hosts, include)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("len=%d", len(got))
	}
	if got[0].Subject != "Standup" {
		t.Fatalf("subject=%q", got[0].Subject)
	}
	if got[0].JoinURL != "https://trueconf.x.com/c/1" {
		t.Fatalf("join=%q", got[0].JoinURL)
	}
	if refs[0].ID != "ABC123" || refs[0].ChangeKey != "X" {
		t.Fatalf("refs=%+v", refs[0])
	}
}

func TestParseFindItemNoJoinURL(t *testing.T) {
	xml := []byte(`<?xml version="1.0"?>
<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/">
 <s:Body>
  <m:FindItemResponse xmlns:m="http://schemas.microsoft.com/exchange/services/2006/messages"
                      xmlns:t="http://schemas.microsoft.com/exchange/services/2006/types">
   <m:ResponseMessages>
    <m:FindItemResponseMessage ResponseClass="Success">
     <m:ResponseCode>NoError</m:ResponseCode>
     <m:RootFolder>
      <t:Items>
       <t:CalendarItem>
        <t:ItemId Id="NOURL" ChangeKey="X"/>
        <t:Subject>Offline</t:Subject>
        <t:Body>agenda only</t:Body>
        <t:Start>2026-07-22T12:05:00Z</t:Start>
        <t:Location>Переговорка 3</t:Location>
        <t:IsCancelled>false</t:IsCancelled>
        <t:MyResponseType>Accept</t:MyResponseType>
       </t:CalendarItem>
      </t:Items>
     </m:RootFolder>
    </m:FindItemResponseMessage>
   </m:ResponseMessages>
  </m:FindItemResponse>
 </s:Body>
</s:Envelope>`)
	hosts := []string{"*.ktalk.ru"}
	include := map[string]struct{}{"Accept": {}}
	got, _, err := parseFindItem(xml, hosts, include)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("len=%d", len(got))
	}
	if got[0].JoinURL != "" {
		t.Fatalf("expected empty JoinURL, got %q", got[0].JoinURL)
	}
}

func TestParseFindItemBodyJoinHost(t *testing.T) {
	xml := []byte(`<?xml version="1.0"?>
<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/">
 <s:Body>
  <m:FindItemResponse xmlns:m="http://schemas.microsoft.com/exchange/services/2006/messages"
                      xmlns:t="http://schemas.microsoft.com/exchange/services/2006/types">
   <m:ResponseMessages>
    <m:FindItemResponseMessage ResponseClass="Success">
     <m:ResponseCode>NoError</m:ResponseCode>
     <m:RootFolder>
      <t:Items>
       <t:CalendarItem>
        <t:ItemId Id="BODY" ChangeKey="X"/>
        <t:Subject>From body</t:Subject>
        <t:Body>join https://x.ktalk.ru/bss</t:Body>
        <t:Start>2026-07-22T12:05:00Z</t:Start>
        <t:Location>Переговорка 3</t:Location>
        <t:IsCancelled>false</t:IsCancelled>
        <t:MyResponseType>Accept</t:MyResponseType>
       </t:CalendarItem>
      </t:Items>
     </m:RootFolder>
    </m:FindItemResponseMessage>
   </m:ResponseMessages>
  </m:FindItemResponse>
 </s:Body>
</s:Envelope>`)
	hosts := []string{"*.ktalk.ru"}
	include := map[string]struct{}{"Accept": {}}
	got, _, err := parseFindItem(xml, hosts, include)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("len=%d", len(got))
	}
	if got[0].JoinURL != "https://x.ktalk.ru/bss" {
		t.Fatalf("join=%q", got[0].JoinURL)
	}
}

func TestParseGetItemBodies(t *testing.T) {
	xml := []byte(`<?xml version="1.0"?>
<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/">
 <s:Body>
  <m:GetItemResponse xmlns:m="http://schemas.microsoft.com/exchange/services/2006/messages"
                     xmlns:t="http://schemas.microsoft.com/exchange/services/2006/types">
   <m:ResponseMessages>
    <m:GetItemResponseMessage ResponseClass="Success">
     <m:ResponseCode>NoError</m:ResponseCode>
     <m:Items>
      <t:CalendarItem>
       <t:ItemId Id="BODY1" ChangeKey="A"/>
       <t:Body BodyType="HTML">&lt;p&gt;see https://nexign.ktalk.ru/bss&lt;/p&gt;</t:Body>
      </t:CalendarItem>
     </m:Items>
    </m:GetItemResponseMessage>
    <m:GetItemResponseMessage ResponseClass="Error">
     <m:MessageText>gone</m:MessageText>
    </m:GetItemResponseMessage>
    <m:GetItemResponseMessage ResponseClass="Success">
     <m:ResponseCode>NoError</m:ResponseCode>
     <m:Items>
      <t:CalendarItem>
       <t:ItemId Id="BODY2" ChangeKey="B"/>
       <t:Body BodyType="Text">plain https://x.ktalk.ru/room</t:Body>
      </t:CalendarItem>
     </m:Items>
    </m:GetItemResponseMessage>
   </m:ResponseMessages>
  </m:GetItemResponse>
 </s:Body>
</s:Envelope>`)
	got, err := parseGetItemBodies(xml)
	if err != nil {
		t.Fatal(err)
	}
	if got["BODY1"] != "<p>see https://nexign.ktalk.ru/bss</p>" {
		t.Fatalf("BODY1=%q", got["BODY1"])
	}
	if got["BODY2"] != "plain https://x.ktalk.ru/room" {
		t.Fatalf("BODY2=%q", got["BODY2"])
	}
	if _, ok := got["missing"]; ok {
		t.Fatal("unexpected missing key")
	}
}

func TestBuildGetItem(t *testing.T) {
	s := buildGetItem([]itemRef{
		{ID: "id1", ChangeKey: "ck&1"},
		{ID: "id2"},
	})
	for _, part := range []string{`Id="id1"`, `ChangeKey="ck&amp;1"`, `Id="id2"`, `item:Body`} {
		if !strings.Contains(s, part) {
			t.Fatalf("missing %q in GetItem SOAP:\n%s", part, s)
		}
	}
}
