package cronutils

import "testing"

func TestValidateTimezone(t *testing.T) {
	if !ValidateTimezone("Asia/Shanghai") {
		t.Fatal("expected valid timezone")
	}

	if ValidateTimezone("invalid/timezone") {
		t.Fatal("expected invalid timezone")
	}
}
