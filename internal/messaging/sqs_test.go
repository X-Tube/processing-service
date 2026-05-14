package messaging

import "testing"

func TestMaskReceiptHandle(t *testing.T) {
	tests := map[string]string{
		"":                           "",
		"short":                      "*****",
		"abcdefghijklmnopqrstuvwxyz": "abcdef...uvwxyz",
	}

	for input, expected := range tests {
		t.Run(input, func(t *testing.T) {
			if got := MaskReceiptHandle(input); got != expected {
				t.Fatalf("expected %q, got %q", expected, got)
			}
		})
	}
}
