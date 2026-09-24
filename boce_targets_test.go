package main

import (
	"reflect"
	"strings"
	"testing"
)

// TestValidateTargetsRejectsLooseCases 覆盖旧的宽松解析会放过、
// 但前端 isValidHost / isValidIPv4 一律拒绝的输入。
func TestValidateTargetsRejectsLooseCases(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{name: "无点主机", input: "abc"},
		{name: "连续点", input: "a..b.com"},
		{name: "空 scheme", input: "http://"},
		{name: "非 http scheme", input: "ftp://x.com"},
		{name: "带假 scheme", input: "ahttp://x.com"},
		{name: "非法端口", input: "example.com:abc"},
		{name: "超长标签", input: strings.Repeat("a", 65) + ".com"},
		{name: "超长 host", input: strings.Repeat("ab.", 90) + "com"},
		{name: "TLD 无字母", input: "a.12"},
		{name: "TLD 单字符", input: "a.c"},
		{name: "标签首连字符", input: "-a.com"},
		{name: "标签尾连字符", input: "a-.com"},
		{name: "标签含下划线", input: "a_b.com"},
		{name: "前导零 IPv4（WHATWG 会归一化，见 normalizeTarget 注释）", input: "01.2.3.4"},
		{name: "越界 IPv4", input: "1.2.3.256"},
		{name: "五段伪 IPv4", input: "1.2.3.4.5"},
		{name: "含 @", input: "user@x.com"},
		// IDN 中文域名：前端经 new URL 转 punycode 后放行，Go 侧拒绝（已知差异，见 normalizeTarget 注释）。
		{name: "IDN 中文域名原形", input: "中国.com"},
	}

	app := &App{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := app.ValidateTargets(tt.input)
			if len(got.Valid) != 0 {
				t.Fatalf("ValidateTargets(%q).Valid = %v, want 空", tt.input, got.Valid)
			}
			if want := []string{tt.input}; !reflect.DeepEqual(got.Invalid, want) {
				t.Fatalf("ValidateTargets(%q).Invalid = %#v, want %#v", tt.input, got.Invalid, want)
			}
			if got.Message == "" {
				t.Fatalf("ValidateTargets(%q).Message = \"\", want 非空中文提示", tt.input)
			}
		})
	}
}

// TestValidateTargetsAcceptsRealCases 断言真实域名 / URL / IPv4 归一化后进 Valid。
func TestValidateTargetsAcceptsRealCases(t *testing.T) {
	tests := []struct {
		name string
		text string
		want []string
	}{
		{name: "裸域名", text: "www.baidu.com", want: []string{"www.baidu.com"}},
		{name: "带路径查询的 URL", text: "https://www.baidu.com/path?q=1", want: []string{"www.baidu.com"}},
		{name: "IPv4", text: "1.2.3.4", want: []string{"1.2.3.4"}},
		{name: "带端口与路径", text: "boce.com:8080/x", want: []string{"boce.com"}},
		{name: "数字开头标签", text: "1.2.3.com", want: []string{"1.2.3.com"}},
		{
			name: "混合分隔符",
			text: "boce.com, example.com；http://foo.test",
			want: []string{"boce.com", "example.com", "foo.test"},
		},
		{
			name: "大小写与去重",
			text: "www.baidu.com\nhttps://WWW.BAIDU.COM/search?q=1",
			want: []string{"www.baidu.com"},
		},
	}

	app := &App{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := app.ValidateTargets(tt.text)
			if !reflect.DeepEqual(got.Valid, tt.want) {
				t.Fatalf("ValidateTargets(%q).Valid = %#v, want %#v", tt.text, got.Valid, tt.want)
			}
			if len(got.Invalid) != 0 {
				t.Fatalf("ValidateTargets(%q).Invalid = %#v, want 空", tt.text, got.Invalid)
			}
			if got.Message != "" {
				t.Fatalf("ValidateTargets(%q).Message = %q, want 空", tt.text, got.Message)
			}
		})
	}
}

// TestNormalizeTargetsRejectsInlineJunk 断言带空白 / @ 的单条输入进 Invalid，
// 纯空白项按前端 filter(Boolean) 的约定直接忽略（parseTargetsText 会按空白切分，
// 所以这条单条路径只能从 normalizeTargets 覆盖）。
func TestNormalizeTargetsRejectsInlineJunk(t *testing.T) {
	got := normalizeTargets([]string{"a b.com", "x\ty.com", "user@x.com", "   ", "\ufeffok.com"})
	want := TargetList{
		Valid:   []string{"ok.com"},
		Invalid: []string{"a b.com", "x\ty.com", "user@x.com"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("normalizeTargets() = %#v, want %#v", got, want)
	}
}

// TestParseTargetsTextReportsInvalid 断言非法项不再被静默丢弃，而是原样进 Invalid。
func TestParseTargetsTextReportsInvalid(t *testing.T) {
	content := "\ufeffhttps://www.baidu.com/path\nabc\nboce.com, a..b.com；http://foo.test\nftp://x.com"
	got := parseTargetsText(content)

	wantValid := []string{"www.baidu.com", "boce.com", "foo.test"}
	wantInvalid := []string{"abc", "a..b.com", "ftp://x.com"}
	if !reflect.DeepEqual(got.Valid, wantValid) {
		t.Fatalf("parseTargetsText().Valid = %#v, want %#v", got.Valid, wantValid)
	}
	if !reflect.DeepEqual(got.Invalid, wantInvalid) {
		t.Fatalf("parseTargetsText().Invalid = %#v, want %#v", got.Invalid, wantInvalid)
	}
}

// TestTargetValidationMessage 断言 Go 侧提示文案与前端现有格式一致。
func TestTargetValidationMessage(t *testing.T) {
	many := TargetList{Valid: []string{"ok.com"}, Invalid: []string{"a", "b", "c", "d", "e"}}
	if got, want := targetValidationMessage("a b c d e ok.com", many), "检测目标格式错误: a、b、c 等 5 条"; got != want {
		t.Fatalf("targetValidationMessage() = %q, want %q", got, want)
	}

	few := TargetList{Valid: []string{"ok.com"}, Invalid: []string{"a", "b"}}
	if got := targetValidationMessage("a b ok.com", few); got != "检测目标格式错误: a、b" {
		t.Fatalf("targetValidationMessage() = %q", got)
	}

	if got := targetValidationMessage("   ", TargetList{}); got != "" {
		t.Fatalf("targetValidationMessage(空白) = %q, want 空", got)
	}
}
