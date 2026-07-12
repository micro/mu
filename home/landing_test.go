package home

import "testing"

func TestFormatPrice(t *testing.T) {
	cases := []struct {
		in   float64
		want string
	}{
		{64152.4, "$64,152"},
		{1818.0, "$1,818"},
		{999.5, "$999.50"},
		{77.46, "$77.46"},
		{1.10, "$1.10"},
		{0.4213, "$0.4213"},
		{4114000, "$4,114,000"},
	}
	for _, c := range cases {
		if got := formatPrice(c.in); got != c.want {
			t.Errorf("formatPrice(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestAddThousands(t *testing.T) {
	cases := map[string]string{
		"1": "1", "12": "12", "123": "123",
		"1234": "1,234", "12345": "12,345",
		"123456": "123,456", "1234567": "1,234,567",
	}
	for in, want := range cases {
		if got := addThousands(in); got != want {
			t.Errorf("addThousands(%q) = %q, want %q", in, got, want)
		}
	}
}
