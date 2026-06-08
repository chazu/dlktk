// Package id mints node identifiers: proquints derived from timestamp-prefixed
// ULIDs. Pronounceable, roughly sortable by creation, stable across machines,
// and friendly for an agent to echo back. The canonical id is the proquint; the
// optional kind prefix (i:/p:/a:) is presentational only (see ibis.PrefixFor).
package id

import (
	"crypto/rand"
	"time"

	"github.com/oklog/ulid/v2"
)

// proquint alphabets: 16 consonants (4 bits) interleaved with 4 vowels (2 bits).
const (
	consonants = "bdfghjklmnprstvz"
	vowels     = "aiou"
)

// New returns a fresh node id: two proquint words (e.g. "kibod-marok").
//
// The words encode ULID bytes [4:8] — the low 16 bits of the millisecond
// timestamp plus the first 16 bits of entropy. That keeps ids unique within a
// session and roughly creation-ordered (the timestamp advances), while staying
// short enough to type and say. Full ULID sortability is not preserved in the
// 4-byte projection; revisit if strict ordering ever matters.
func New() string {
	u := ulid.MustNew(ulid.Timestamp(time.Now()), rand.Reader)
	return Proquint(u[4:8])
}

// Proquint encodes an even-length byte slice as hyphen-separated proquint words,
// one word per 2 bytes (16 bits). A trailing odd byte is ignored.
func Proquint(b []byte) string {
	out := make([]byte, 0, (len(b)/2)*6)
	for i := 0; i+1 < len(b); i += 2 {
		if i > 0 {
			out = append(out, '-')
		}
		n := uint16(b[i])<<8 | uint16(b[i+1])
		out = append(out,
			consonants[(n>>12)&0x0f],
			vowels[(n>>10)&0x03],
			consonants[(n>>6)&0x0f],
			vowels[(n>>4)&0x03],
			consonants[n&0x0f],
		)
	}
	return string(out)
}
