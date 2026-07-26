package ews

import "testing"

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
	got, err := parseFindItem(xml, hosts, include)
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
	got, err := parseFindItem(xml, hosts, include)
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
	got, err := parseFindItem(xml, hosts, include)
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
