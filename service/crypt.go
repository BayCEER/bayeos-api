package service

import (
	"encoding/binary"
	"errors"
	"sync"
)

// Pure Go port of PostgreSQL's contrib/pgcrypto/crypt-des.c (FreeSec by David
// Burren, adapted for FreeBSD by Mark R V Murray). This produces byte-identical
// output to pgcrypto's crypt() for the traditional DES scheme.
//
// The mutable state of the original C implementation (key schedule, salt bits)
// is kept per call, so CryptDES is safe for concurrent use.

const cryptA64 = "./0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"

var cryptIP = [64]byte{
	58, 50, 42, 34, 26, 18, 10, 2, 60, 52, 44, 36, 28, 20, 12, 4,
	62, 54, 46, 38, 30, 22, 14, 6, 64, 56, 48, 40, 32, 24, 16, 8,
	57, 49, 41, 33, 25, 17, 9, 1, 59, 51, 43, 35, 27, 19, 11, 3,
	61, 53, 45, 37, 29, 21, 13, 5, 63, 55, 47, 39, 31, 23, 15, 7,
}

var cryptKeyPerm = [56]byte{
	57, 49, 41, 33, 25, 17, 9, 1, 58, 50, 42, 34, 26, 18,
	10, 2, 59, 51, 43, 35, 27, 19, 11, 3, 60, 52, 44, 36,
	63, 55, 47, 39, 31, 23, 15, 7, 62, 54, 46, 38, 30, 22,
	14, 6, 61, 53, 45, 37, 29, 21, 13, 5, 28, 20, 12, 4,
}

var cryptKeyShifts = [16]byte{
	1, 1, 2, 2, 2, 2, 2, 2, 1, 2, 2, 2, 2, 2, 2, 1,
}

var cryptCompPerm = [48]byte{
	14, 17, 11, 24, 1, 5, 3, 28, 15, 6, 21, 10,
	23, 19, 12, 4, 26, 8, 16, 7, 27, 20, 13, 2,
	41, 52, 31, 37, 47, 55, 30, 40, 51, 45, 33, 48,
	44, 49, 39, 56, 34, 53, 46, 42, 50, 36, 29, 32,
}

var cryptSbox = [8][64]byte{
	{
		14, 4, 13, 1, 2, 15, 11, 8, 3, 10, 6, 12, 5, 9, 0, 7,
		0, 15, 7, 4, 14, 2, 13, 1, 10, 6, 12, 11, 9, 5, 3, 8,
		4, 1, 14, 8, 13, 6, 2, 11, 15, 12, 9, 7, 3, 10, 5, 0,
		15, 12, 8, 2, 4, 9, 1, 7, 5, 11, 3, 14, 10, 0, 6, 13,
	},
	{
		15, 1, 8, 14, 6, 11, 3, 4, 9, 7, 2, 13, 12, 0, 5, 10,
		3, 13, 4, 7, 15, 2, 8, 14, 12, 0, 1, 10, 6, 9, 11, 5,
		0, 14, 7, 11, 10, 4, 13, 1, 5, 8, 12, 6, 9, 3, 2, 15,
		13, 8, 10, 1, 3, 15, 4, 2, 11, 6, 7, 12, 0, 5, 14, 9,
	},
	{
		10, 0, 9, 14, 6, 3, 15, 5, 1, 13, 12, 7, 11, 4, 2, 8,
		13, 7, 0, 9, 3, 4, 6, 10, 2, 8, 5, 14, 12, 11, 15, 1,
		13, 6, 4, 9, 8, 15, 3, 0, 11, 1, 2, 12, 5, 10, 14, 7,
		1, 10, 13, 0, 6, 9, 8, 7, 4, 15, 14, 3, 11, 5, 2, 12,
	},
	{
		7, 13, 14, 3, 0, 6, 9, 10, 1, 2, 8, 5, 11, 12, 4, 15,
		13, 8, 11, 5, 6, 15, 0, 3, 4, 7, 2, 12, 1, 10, 14, 9,
		10, 6, 9, 0, 12, 11, 7, 13, 15, 1, 3, 14, 5, 2, 8, 4,
		3, 15, 0, 6, 10, 1, 13, 8, 9, 4, 5, 11, 12, 7, 2, 14,
	},
	{
		2, 12, 4, 1, 7, 10, 11, 6, 8, 5, 3, 15, 13, 0, 14, 9,
		14, 11, 2, 12, 4, 7, 13, 1, 5, 0, 15, 10, 3, 9, 8, 6,
		4, 2, 1, 11, 10, 13, 7, 8, 15, 9, 12, 5, 6, 3, 0, 14,
		11, 8, 12, 7, 1, 14, 2, 13, 6, 15, 0, 9, 10, 4, 5, 3,
	},
	{
		12, 1, 10, 15, 9, 2, 6, 8, 0, 13, 3, 4, 14, 7, 5, 11,
		10, 15, 4, 2, 7, 12, 9, 5, 6, 1, 13, 14, 0, 11, 3, 8,
		9, 14, 15, 5, 2, 8, 12, 3, 7, 0, 4, 10, 1, 13, 11, 6,
		4, 3, 2, 12, 9, 5, 15, 10, 11, 14, 1, 7, 6, 0, 8, 13,
	},
	{
		4, 11, 2, 14, 15, 0, 8, 13, 3, 12, 9, 7, 5, 10, 6, 1,
		13, 0, 11, 7, 4, 9, 1, 10, 14, 3, 5, 12, 2, 15, 8, 6,
		1, 4, 11, 13, 12, 3, 7, 14, 10, 15, 6, 8, 0, 5, 9, 2,
		6, 11, 13, 8, 1, 4, 10, 7, 9, 5, 0, 15, 14, 2, 3, 12,
	},
	{
		13, 2, 8, 4, 6, 15, 11, 1, 10, 9, 3, 14, 5, 0, 12, 7,
		1, 15, 13, 8, 10, 3, 7, 4, 12, 5, 6, 11, 0, 14, 9, 2,
		7, 11, 4, 1, 9, 12, 14, 2, 0, 6, 10, 13, 15, 3, 5, 8,
		2, 1, 14, 7, 4, 10, 8, 13, 15, 12, 9, 0, 3, 5, 6, 11,
	},
}

var cryptPbox = [32]byte{
	16, 7, 20, 21, 29, 12, 28, 17, 1, 15, 23, 26, 5, 18, 31, 10,
	2, 8, 24, 14, 32, 27, 3, 9, 19, 13, 30, 6, 22, 11, 4, 25,
}

var cryptBits32 = [32]uint32{
	0x80000000, 0x40000000, 0x20000000, 0x10000000,
	0x08000000, 0x04000000, 0x02000000, 0x01000000,
	0x00800000, 0x00400000, 0x00200000, 0x00100000,
	0x00080000, 0x00040000, 0x00020000, 0x00010000,
	0x00008000, 0x00004000, 0x00002000, 0x00001000,
	0x00000800, 0x00000400, 0x00000200, 0x00000100,
	0x00000080, 0x00000040, 0x00000020, 0x00000010,
	0x00000008, 0x00000004, 0x00000002, 0x00000001,
}

var cryptBits8 = [8]byte{0x80, 0x40, 0x20, 0x10, 0x08, 0x04, 0x02, 0x01}

var (
	desInitOnce sync.Once

	desInitPerm     [64]byte
	desFinalPerm    [64]byte
	desInvKeyPerm   [64]byte
	desInvCompPerm  [56]byte
	desUSbox        [8][64]byte
	desMSbox        [4][4096]byte
	desPsbox        [4][256]uint32
	desIPMaskL      [8][256]uint32
	desIPMaskR      [8][256]uint32
	desFPMaskL      [8][256]uint32
	desFPMaskR      [8][256]uint32
	desKeyPermMaskL [8][128]uint32
	desKeyPermMaskR [8][128]uint32
	desCompMaskL    [8][128]uint32
	desCompMaskR    [8][128]uint32
	desUnPbox       [32]byte
)

func desInit() {
	for i := 0; i < 8; i++ {
		for j := 0; j < 64; j++ {
			b := (j & 0x20) | ((j & 1) << 4) | ((j >> 1) & 0xf)
			desUSbox[i][j] = cryptSbox[i][b]
		}
	}

	for b := 0; b < 4; b++ {
		for i := 0; i < 64; i++ {
			for j := 0; j < 64; j++ {
				desMSbox[b][(i<<6)|j] = (desUSbox[b<<1][i] << 4) | desUSbox[(b<<1)+1][j]
			}
		}
	}

	for i := 0; i < 64; i++ {
		desFinalPerm[i] = cryptIP[i] - 1
		desInitPerm[desFinalPerm[i]] = byte(i)
		desInvKeyPerm[i] = 255
	}

	for i := 0; i < 56; i++ {
		desInvKeyPerm[cryptKeyPerm[i]-1] = byte(i)
		desInvCompPerm[i] = 255
	}

	for i := 0; i < 48; i++ {
		desInvCompPerm[cryptCompPerm[i]-1] = byte(i)
	}

	for k := 0; k < 8; k++ {
		for i := 0; i < 256; i++ {
			desIPMaskL[k][i] = 0
			desIPMaskR[k][i] = 0
			desFPMaskL[k][i] = 0
			desFPMaskR[k][i] = 0
			for j := 0; j < 8; j++ {
				inbit := 8*k + j
				if i&int(cryptBits8[j]) != 0 {
					if obit := desInitPerm[inbit]; obit < 32 {
						desIPMaskL[k][i] |= cryptBits32[obit]
					} else {
						desIPMaskR[k][i] |= cryptBits32[obit-32]
					}
					if obit := desFinalPerm[inbit]; obit < 32 {
						desFPMaskL[k][i] |= cryptBits32[obit]
					} else {
						desFPMaskR[k][i] |= cryptBits32[obit-32]
					}
				}
			}
		}
		for i := 0; i < 128; i++ {
			desKeyPermMaskL[k][i] = 0
			desKeyPermMaskR[k][i] = 0
			for j := 0; j < 7; j++ {
				inbit := 8*k + j
				if i&int(cryptBits8[j+1]) != 0 {
					obit := desInvKeyPerm[inbit]
					if obit == 255 {
						continue
					}
					if obit < 28 {
						desKeyPermMaskL[k][i] |= cryptBits32[4+int(obit)]
					} else {
						desKeyPermMaskR[k][i] |= cryptBits32[4+int(obit)-28]
					}
				}
			}
			desCompMaskL[k][i] = 0
			desCompMaskR[k][i] = 0
			for j := 0; j < 7; j++ {
				inbit := 7*k + j
				if i&int(cryptBits8[j+1]) != 0 {
					obit := desInvCompPerm[inbit]
					if obit == 255 {
						continue
					}
					if obit < 24 {
						desCompMaskL[k][i] |= cryptBits32[8+int(obit)]
					} else {
						desCompMaskR[k][i] |= cryptBits32[8+int(obit)-24]
					}
				}
			}
		}
	}

	for i := 0; i < 32; i++ {
		desUnPbox[cryptPbox[i]-1] = byte(i)
	}

	for b := 0; b < 4; b++ {
		for i := 0; i < 256; i++ {
			desPsbox[b][i] = 0
			for j := 0; j < 8; j++ {
				if i&int(cryptBits8[j]) != 0 {
					desPsbox[b][i] |= cryptBits32[desUnPbox[8*b+j]]
				}
			}
		}
	}
}

func asciiToBin(ch byte) uint32 {
	switch {
	case ch > 'z':
		return 0
	case ch >= 'a':
		return uint32(ch - 'a' + 38)
	case ch > 'Z':
		return 0
	case ch >= 'A':
		return uint32(ch - 'A' + 12)
	case ch > '9':
		return 0
	case ch >= '.':
		return uint32(ch - '.')
	default:
		return 0
	}
}

type desState struct {
	saltbits uint32
	enKeysL  [16]uint32
	enKeysR  [16]uint32
	deKeysL  [16]uint32
	deKeysR  [16]uint32
}

func (d *desState) setupSalt(salt uint32) {
	d.saltbits = 0
	saltbit := uint32(1)
	obit := uint32(0x800000)
	for i := 0; i < 24; i++ {
		if salt&saltbit != 0 {
			d.saltbits |= obit
		}
		saltbit <<= 1
		obit >>= 1
	}
}

func (d *desState) setKey(key [8]byte) {
	rawkey0 := binary.BigEndian.Uint32(key[0:4])
	rawkey1 := binary.BigEndian.Uint32(key[4:8])

	k0 := desKeyPermMaskL[0][rawkey0>>25] |
		desKeyPermMaskL[1][(rawkey0>>17)&0x7f] |
		desKeyPermMaskL[2][(rawkey0>>9)&0x7f] |
		desKeyPermMaskL[3][(rawkey0>>1)&0x7f] |
		desKeyPermMaskL[4][rawkey1>>25] |
		desKeyPermMaskL[5][(rawkey1>>17)&0x7f] |
		desKeyPermMaskL[6][(rawkey1>>9)&0x7f] |
		desKeyPermMaskL[7][(rawkey1>>1)&0x7f]
	k1 := desKeyPermMaskR[0][rawkey0>>25] |
		desKeyPermMaskR[1][(rawkey0>>17)&0x7f] |
		desKeyPermMaskR[2][(rawkey0>>9)&0x7f] |
		desKeyPermMaskR[3][(rawkey0>>1)&0x7f] |
		desKeyPermMaskR[4][rawkey1>>25] |
		desKeyPermMaskR[5][(rawkey1>>17)&0x7f] |
		desKeyPermMaskR[6][(rawkey1>>9)&0x7f] |
		desKeyPermMaskR[7][(rawkey1>>1)&0x7f]

	shifts := uint(0)
	for round := 0; round < 16; round++ {
		shifts += uint(cryptKeyShifts[round])
		t0 := (k0 << shifts) | (k0 >> (28 - shifts))
		t1 := (k1 << shifts) | (k1 >> (28 - shifts))

		l := desCompMaskL[0][(t0>>21)&0x7f] |
			desCompMaskL[1][(t0>>14)&0x7f] |
			desCompMaskL[2][(t0>>7)&0x7f] |
			desCompMaskL[3][t0&0x7f] |
			desCompMaskL[4][(t1>>21)&0x7f] |
			desCompMaskL[5][(t1>>14)&0x7f] |
			desCompMaskL[6][(t1>>7)&0x7f] |
			desCompMaskL[7][t1&0x7f]
		r := desCompMaskR[0][(t0>>21)&0x7f] |
			desCompMaskR[1][(t0>>14)&0x7f] |
			desCompMaskR[2][(t0>>7)&0x7f] |
			desCompMaskR[3][t0&0x7f] |
			desCompMaskR[4][(t1>>21)&0x7f] |
			desCompMaskR[5][(t1>>14)&0x7f] |
			desCompMaskR[6][(t1>>7)&0x7f] |
			desCompMaskR[7][t1&0x7f]

		d.deKeysL[15-round] = l
		d.enKeysL[round] = l
		d.deKeysR[15-round] = r
		d.enKeysR[round] = r
	}
}

func (d *desState) doDES(lIn, rIn uint32, count int) (uint32, uint32) {
	var keysL, keysR *[16]uint32
	if count > 0 {
		keysL = &d.enKeysL
		keysR = &d.enKeysR
	} else {
		count = -count
		keysL = &d.deKeysL
		keysR = &d.deKeysR
	}

	l := desIPMaskL[0][lIn>>24] |
		desIPMaskL[1][(lIn>>16)&0xff] |
		desIPMaskL[2][(lIn>>8)&0xff] |
		desIPMaskL[3][lIn&0xff] |
		desIPMaskL[4][rIn>>24] |
		desIPMaskL[5][(rIn>>16)&0xff] |
		desIPMaskL[6][(rIn>>8)&0xff] |
		desIPMaskL[7][rIn&0xff]
	r := desIPMaskR[0][lIn>>24] |
		desIPMaskR[1][(lIn>>16)&0xff] |
		desIPMaskR[2][(lIn>>8)&0xff] |
		desIPMaskR[3][lIn&0xff] |
		desIPMaskR[4][rIn>>24] |
		desIPMaskR[5][(rIn>>16)&0xff] |
		desIPMaskR[6][(rIn>>8)&0xff] |
		desIPMaskR[7][rIn&0xff]

	var f uint32
	for ; count > 0; count-- {
		for round := 0; round < 16; round++ {
			r48l := ((r & 0x00000001) << 23) |
				((r & 0xf8000000) >> 9) |
				((r & 0x1f800000) >> 11) |
				((r & 0x01f80000) >> 13) |
				((r & 0x001f8000) >> 15)
			r48r := ((r & 0x0001f800) << 7) |
				((r & 0x00001f80) << 5) |
				((r & 0x000001f8) << 3) |
				((r & 0x0000001f) << 1) |
				((r & 0x80000000) >> 31)

			f = (r48l ^ r48r) & d.saltbits
			r48l ^= f ^ keysL[round]
			r48r ^= f ^ keysR[round]

			f = desPsbox[0][desMSbox[0][r48l>>12]] |
				desPsbox[1][desMSbox[1][r48l&0xfff]] |
				desPsbox[2][desMSbox[2][r48r>>12]] |
				desPsbox[3][desMSbox[3][r48r&0xfff]]

			f ^= l
			l = r
			r = f
		}
		r = l
		l = f
	}

	lOut := desFPMaskL[0][l>>24] |
		desFPMaskL[1][(l>>16)&0xff] |
		desFPMaskL[2][(l>>8)&0xff] |
		desFPMaskL[3][l&0xff] |
		desFPMaskL[4][r>>24] |
		desFPMaskL[5][(r>>16)&0xff] |
		desFPMaskL[6][(r>>8)&0xff] |
		desFPMaskL[7][r&0xff]
	rOut := desFPMaskR[0][l>>24] |
		desFPMaskR[1][(l>>16)&0xff] |
		desFPMaskR[2][(l>>8)&0xff] |
		desFPMaskR[3][l&0xff] |
		desFPMaskR[4][r>>24] |
		desFPMaskR[5][(r>>16)&0xff] |
		desFPMaskR[6][(r>>8)&0xff] |
		desFPMaskR[7][r&0xff]

	return lOut, rOut
}

// CryptDES generates a traditional crypt(3) DES hash, byte-compatible with
// PostgreSQL pgcrypto's crypt(password, salt) for a 2-character salt drawn
// from [a-zA-Z0-9./].
func CryptDES(password, salt string) (string, error) {
	if len(salt) < 2 {
		return "", errors.New("invalid salt")
	}
	desInitOnce.Do(desInit)

	var keybuf [8]byte
	keyIdx := 0
	for i := 0; i < 8; i++ {
		var c byte
		if keyIdx < len(password) {
			c = password[keyIdx]
		}
		keybuf[i] = c << 1
		if c != 0 {
			keyIdx++
		}
	}

	var d desState
	d.setKey(keybuf)

	saltVal := (asciiToBin(salt[1]) << 6) | asciiToBin(salt[0])
	d.setupSalt(saltVal)

	r0, r1 := d.doDES(0, 0, 25)

	s0 := salt[0]
	s1 := salt[1]
	if s1 == 0 {
		s1 = s0
	}

	out := make([]byte, 0, 13)
	out = append(out, s0, s1)

	l := r0 >> 8
	out = append(out,
		cryptA64[(l>>18)&0x3f],
		cryptA64[(l>>12)&0x3f],
		cryptA64[(l>>6)&0x3f],
		cryptA64[l&0x3f],
	)

	l = (r0 << 16) | ((r1 >> 16) & 0xffff)
	out = append(out,
		cryptA64[(l>>18)&0x3f],
		cryptA64[(l>>12)&0x3f],
		cryptA64[(l>>6)&0x3f],
		cryptA64[l&0x3f],
	)

	l = r1 << 2
	out = append(out,
		cryptA64[(l>>12)&0x3f],
		cryptA64[(l>>6)&0x3f],
		cryptA64[l&0x3f],
	)

	return string(out), nil
}
