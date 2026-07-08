package rs485

import "testing"

// TestValidResponse defends the RS485 ack-validation contract used during
// port auto-discovery. validResponse returns true iff:
//   - len(data) >= n AND n >= 2
//   - data[0] matches one of prefixes
//   - low byte of sum(data[0..n-2]) == data[n-1]
//
// Light, alloff, and boiler acks are all 8 bytes (checksum at index 7). All
// byte sequences below are real device-log captures; every expected checksum is
// verified by hand in the case comment.
func TestValidResponse(t *testing.T) {
	tests := []struct {
		name     string
		data     []byte
		n        int
		prefixes []byte
		want     bool
	}{
		{
			// sum[0..6] = B0+00+02+00+00+00+00 = 0xB2, data[7]=0xB2 -> match
			name:     "light ack (light 2, off), 8-byte, prefix B0",
			data:     []byte{0xB0, 0x00, 0x02, 0x00, 0x00, 0x00, 0x00, 0xB2},
			n:        8,
			prefixes: []byte{0xB0},
			want:     true,
		},
		{
			// sum[0..6] = B0+01+03+00+00+00+00 = 0xB4, data[7]=0xB4 -> match
			name:     "light ack (light 3, on), 8-byte, prefix B0",
			data:     []byte{0xB0, 0x01, 0x03, 0x00, 0x00, 0x00, 0x00, 0xB4},
			n:        8,
			prefixes: []byte{0xB0},
			want:     true,
		},
		{
			// sum[0..6] = A0+01+01+00+00+15+00 = 0xB7, data[7]=0xB7 -> match
			name:     "alloff ack, 8-byte, prefix A0",
			data:     []byte{0xA0, 0x01, 0x01, 0x00, 0x00, 0x15, 0x00, 0xB7},
			n:        8,
			prefixes: []byte{0xA0},
			want:     true,
		},
		{
			// sum[0..6] = 82+81+01+27+15+00+00 = 0x140 -> low byte 0x40,
			// data[7]=0x40 -> match; prefix 0x82 is the first of {0x82,0x84}
			name:     "boiler ack, 8-byte, prefixes 82/84",
			data:     []byte{0x82, 0x81, 0x01, 0x27, 0x15, 0x00, 0x00, 0x40},
			n:        8,
			prefixes: []byte{0x82, 0x84},
			want:     true,
		},
		{
			// Regression for the current bug class: an 8-byte light ack whose
			// checksum byte is wrong must be rejected. sum[0..6]=0xB2 but
			// data[7]=0xFF -> checksum mismatch -> false. A validator that
			// skipped the checksum compare would wrongly accept this frame.
			name:     "regression: 8-byte light ack, wrong checksum (n=8)",
			data:     []byte{0xB0, 0x00, 0x02, 0x00, 0x00, 0x00, 0x00, 0xFF},
			n:        8,
			prefixes: []byte{0xB0},
			want:     false,
		},
		{
			// Generic length-parameterized case (not a light frame): proves
			// validResponse works for arbitrary n. sum[0..2] = 10+20+30 = 0x60,
			// data[3]=0x60 -> match at n=4.
			name:     "generic n=4 checksum",
			data:     []byte{0x10, 0x20, 0x30, 0x60},
			n:        4,
			prefixes: []byte{0x10},
			want:     true,
		},
		{
			// Valid boiler bytes/checksum, but prefix 0xA0 does not match
			// data[0]=0x82 -> false
			name:     "wrong prefix",
			data:     []byte{0x82, 0x81, 0x01, 0x27, 0x15, 0x00, 0x00, 0x40},
			n:        8,
			prefixes: []byte{0xA0},
			want:     false,
		},
		{
			// len(data)=2 < n=8 -> false
			name:     "too short",
			data:     []byte{0xB0, 0x00},
			n:        8,
			prefixes: []byte{0xB0},
			want:     false,
		},
		{
			// n=1 < 2 guard -> false. Bytes chosen so the guard has teeth: if a
			// maintainer weakened/removed the guard, the sum loop over [0..n-2]
			// = [0..-1] runs zero times (sum=0), the prefix 0x00 matches
			// data[0]=0x00, and it would compare sum(0) == data[n-1] = data[0]
			// = 0x00 -> wrongly true. The n<2 guard forces false first.
			name:     "n<2 guard",
			data:     []byte{0x00, 0x00},
			n:        1,
			prefixes: []byte{0x00},
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := validResponse(tt.data, tt.n, tt.prefixes...); got != tt.want {
				t.Errorf("validResponse(% x, %d, % x) = %v, want %v",
					tt.data, tt.n, tt.prefixes, got, tt.want)
			}
		})
	}
}
