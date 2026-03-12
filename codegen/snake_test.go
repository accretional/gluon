package codegen

import "testing"

func TestToSnakeCase(t *testing.T) {
	tests := []struct{ in, want string }{
		{"KVStore", "kv_store"},
		{"ID", "id"},
		{"HTTPServer", "http_server"},
		{"GetHTTPResponse", "get_http_response"},
		{"ItemService", "item_service"},
		{"Simple", "simple"},
		{"ABC", "abc"},
		{"getItem", "get_item"},
		{"HTMLParser", "html_parser"},
	}
	for _, tt := range tests {
		got := toSnakeCase(tt.in)
		if got != tt.want {
			t.Errorf("toSnakeCase(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
