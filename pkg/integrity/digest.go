// Copyright (c) 2020, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the LICENSE.md file
// distributed with the sources of this project regarding your rights to use or distribute this
// software.

package integrity

import (
	"bytes"
	"crypto"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/sylabs/sif/pkg/sif"
)

var (
	errHashUnavailable = errors.New("hash algorithm unavailable")
	errHashUnsupported = errors.New("hash algorithm unsupported")
	errDigestMalformed = errors.New("digest malformed")
)

var supportedAlgorithms = map[crypto.Hash]string{
	crypto.SHA1:   "sha1",
	crypto.SHA224: "sha224",
	crypto.SHA256: "sha256",
	crypto.SHA384: "sha384",
	crypto.SHA512: "sha512",
}

// hashValue calculates a digest by applying hash function h to the contents read from r. If h is
// not available, errHashUnavailable is returned.
func hashValue(h crypto.Hash, r io.Reader) ([]byte, error) {
	if !h.Available() {
		return nil, errHashUnavailable
	}

	w := h.New()
	if _, err := io.Copy(w, r); err != nil {
		return nil, err
	}
	return w.Sum(nil), nil
}

type digest struct {
	hash  crypto.Hash
	value []byte
}

// newDigest returns a new digest. If h is not supported, errHashUnsupported is returned. If digest
// is malformed, errDigestMalformed is returned.
func newDigest(h crypto.Hash, value []byte) (digest, error) {
	if _, ok := supportedAlgorithms[h]; !ok {
		return digest{}, errHashUnsupported
	}

	if len(value) != h.Size() {
		return digest{}, errDigestMalformed
	}

	return digest{h, value}, nil
}

// newDigestReader returns a new digest calculated by applying h to r.
func newDigestReader(h crypto.Hash, r io.Reader) (digest, error) {
	value, err := hashValue(h, r)
	if err != nil {
		return digest{}, err
	}
	return newDigest(h, value)
}

// hashType converts ht into a crypto.Hash value.
func hashType(ht sif.Hashtype) (crypto.Hash, error) {
	switch ht {
	case sif.HashSHA256:
		return crypto.SHA256, nil
	case sif.HashSHA384:
		return crypto.SHA384, nil
	case sif.HashSHA512:
		return crypto.SHA512, nil
	}
	return 0, errHashUnsupported
}

// newLegacyDigest parses the plaintext contained in legacy signatures, returning a digest
// representing the contents.
func newLegacyDigest(ht sif.Hashtype, b []byte) (digest, error) {
	b = bytes.TrimPrefix(b, []byte("SIFHASH:\n"))
	b = bytes.TrimSuffix(b, []byte("\n"))

	// Convert hash type.
	h, err := hashType(ht)
	if err != nil {
		return digest{}, err
	}

	// Decode hex input.
	value := make([]byte, hex.DecodedLen(len(b)))
	if _, err := hex.Decode(value, b); err != nil {
		return digest{}, err
	}

	return newDigest(h, value)
}

// matches returns whether the digest in d matches r.
func (d digest) matches(r io.Reader) (bool, error) {
	value, err := hashValue(d.hash, r)
	if err != nil {
		return false, err
	}
	return bytes.Equal(d.value, value), nil
}

// MarshalJSON marshals d into string of format "alg:value".
func (d digest) MarshalJSON() ([]byte, error) {
	n, ok := supportedAlgorithms[d.hash]
	if !ok {
		return nil, errHashUnsupported
	}
	return json.Marshal(fmt.Sprintf("%s:%x", n, d.value))
}

// UnmarshalJSON unmarshals d from a string of format "alg:value".
func (d *digest) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return fmt.Errorf("%w: %v", errDigestMalformed, err)
	}

	parts := strings.Split(s, ":")
	if len(parts) != 2 {
		return errDigestMalformed
	}
	name := parts[0]
	value := parts[1]

	v, err := hex.DecodeString(value)
	if err != nil {
		return fmt.Errorf("%w: %v", errDigestMalformed, err)
	}

	for h, n := range supportedAlgorithms {
		if n == name {
			digest, err := newDigest(h, v)
			if err != nil {
				return err
			}
			*d = digest
			return nil
		}
	}
	return errHashUnsupported
}
