package dingtalk

import (
	"testing"
	"time"
)

// 真实抓包结构：机器人回答是 interactiveCard，正文在 cardContent 里。
const cardQuotePayload = `{
  "msgtype": "text",
  "text": {
    "content": "/sub 引用测试2",
    "isReplyMsg": true,
    "repliedMsg": {
      "createdAt": 1793174446951,
      "senderId": "$:LWCP_v1:$abc",
      "msgType": "interactiveCard",
      "msgId": "msgee6PTPgwcqRXqJgYphGqDg==",
      "content": {
        "cardContent": [
          {"children": [
            {"elementType": "TEXT", "value": "氪金联盟（Kr）是"},
            {"elementType": "TEXT", "value": "商家经营后台"}
          ]}
        ]
      }
    }
  },
  "originalMsgId": "msgee6PTPgwcqRXqJgYphGqDg=="
}`

const textQuotePayload = `{
  "msgtype": "text",
  "text": {
    "content": "/sub 文本引用",
    "repliedMsg": {"msgId": "msg-1", "createdAt": 1793174400000, "content": "上一轮的问题"}
  }
}`

func TestParseDingTalkQuote_CardReply(t *testing.T) {
	quote := parseDingTalkQuote(cardQuotePayload)
	if quote == nil {
		t.Fatal("卡片引用应能解析")
	}
	if quote.MessageID != "msgee6PTPgwcqRXqJgYphGqDg==" {
		t.Fatalf("msgId 不对: %s", quote.MessageID)
	}
	if quote.CreatedAt.UnixMilli() != 1793174446951 {
		t.Fatalf("createdAt 不对: %v", quote.CreatedAt)
	}
	if quote.Content != "氪金联盟（Kr）是商家经营后台" {
		t.Fatalf("正文提取不对: %q", quote.Content)
	}
}

func TestParseDingTalkQuote_TextReply(t *testing.T) {
	quote := parseDingTalkQuote(textQuotePayload)
	if quote == nil {
		t.Fatal("文本引用应能解析")
	}
	if quote.MessageID != "msg-1" || quote.Content != "上一轮的问题" {
		t.Fatalf("解析结果不对: %+v", quote)
	}
	if !quote.CreatedAt.Equal(time.UnixMilli(1793174400000)) {
		t.Fatalf("createdAt 不对: %v", quote.CreatedAt)
	}
}

func TestParseDingTalkQuote_NotAReply(t *testing.T) {
	if quote := parseDingTalkQuote(`{"msgtype":"text","text":{"content":"/sub 无引用"}}`); quote != nil {
		t.Fatalf("非引用消息不该产出 quote: %+v", quote)
	}
}
