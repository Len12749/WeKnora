package dingtalk

import "testing"

func TestHardenLineBreaks(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "段落内换行补硬换行",
			in:   "第一行\n第二行",
			want: "第一行  \n第二行  ",
		},
		{
			name: "空行保留为空行",
			in:   "**标题**\n\n· /help — 用法",
			want: "**标题**  \n\n· /help — 用法  ",
		},
		{
			name: "代码块内部原样保留",
			in:   "示例：\n```go\nfunc a() {\n\tb()\n}\n```\n结束",
			want: "示例：  \n```go\nfunc a() {\n\tb()\n}\n```\n结束  ",
		},
		{
			name: "已是硬换行不重复补",
			in:   "第一行  \n第二行",
			want: "第一行  \n第二行  ",
		},
		{
			name: "CRLF 归一",
			in:   "第一行\r\n第二行",
			want: "第一行  \n第二行  ",
		},
		{
			name: "空内容",
			in:   "",
			want: "",
		},
		{
			name: "波浪线围栏同样跳过",
			in:   "说明\n~~~\ncode\n~~~",
			want: "说明  \n~~~\ncode\n~~~",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := hardenLineBreaks(c.in); got != c.want {
				t.Fatalf("hardenLineBreaks(%q) = %q, want %q", c.in, got, c.want)
			}
			// 幂等：再跑一次结果不变
			got := hardenLineBreaks(c.in)
			if again := hardenLineBreaks(got); again != got {
				t.Fatalf("非幂等：%q → %q", got, again)
			}
		})
	}
}
