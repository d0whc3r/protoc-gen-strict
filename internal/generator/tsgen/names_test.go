package tsgen

import "testing"

// TestLocalName pins the property names protoc-gen-es declares. The overlay
// indexes into the generated type by these, so a name it gets wrong is either a
// syntax error or a property that does not exist.
func TestLocalName(t *testing.T) {
	tests := []struct{ proto, want string }{
		{"user_id", "userId"},
		{"id", "id"},
		{"http_response_code", "httpResponseCode"},
		{"port_2_name", "port2Name"},
		{"metric_1st", "metric1st"}, // a digit clears the pending capital
		{"constructor", "constructor$"},
		{"toJSON", "toJSON$"}, // proto allows it; protoCamelCase leaves it alone
		{"value_of", "valueOf$"},
	}
	for _, test := range tests {
		if got := localName(test.proto); got != test.want {
			t.Errorf("localName(%q) = %q, want %q", test.proto, got, test.want)
		}
	}
}

// TestMethodName covers the RPC property, which protobuf-es lowercases without
// any snake_case conversion.
func TestMethodName(t *testing.T) {
	tests := []struct{ proto, want string }{
		{"GetUser", "getUser"},
		{"Get_User", "get_User"},
		{"ToString", "toString$"},
	}
	for _, test := range tests {
		if got := methodName(test.proto); got != test.want {
			t.Errorf("methodName(%q) = %q, want %q", test.proto, got, test.want)
		}
	}
}
