package main

type DeviceStatus struct {
	Device string
	State  string
}

func hexStringToBytes(hex string) []byte {
	if len(hex)%2 != 0 {
		return nil
	}
	bytes := make([]byte, len(hex)/2)
	for i := 0; i < len(hex); i += 2 {
		high := hexCharToByte(hex[i])
		low := hexCharToByte(hex[i+1])
		if high == 255 || low == 255 {
			return nil
		}
		bytes[i/2] = high<<4 | low
	}
	return bytes
}

func hexCharToByte(c byte) byte {
	switch {
	case '0' <= c && c <= '9':
		return c - '0'
	case 'a' <= c && c <= 'f':
		return c - 'a' + 10
	case 'A' <= c && c <= 'F':
		return c - 'A' + 10
	default:
		return 255
	}
}
