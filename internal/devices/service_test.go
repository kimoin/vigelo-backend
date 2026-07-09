package devices

import "testing"

func TestParseQRPayload(t *testing.T) {
	payload := "device_id=860123456789012&key=000102030405060708090a0b0c0d0e0f"
	if got := ParseDeviceID(payload); got != "860123456789012" {
		t.Fatalf("device id = %q", got)
	}
	if got := ParseEnrollmentSecret(payload); got != "000102030405060708090a0b0c0d0e0f" {
		t.Fatalf("secret = %q", got)
	}
}

func TestNormalizeEnrollmentSecret(t *testing.T) {
	got := NormalizeEnrollmentSecret("00 01 02 03 04 05 06 07 08 09 0a 0b 0c 0d 0e 0f")
	want := "000102030405060708090a0b0c0d0e0f"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}
