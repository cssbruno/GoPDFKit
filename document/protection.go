// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package document

import (
	"crypto/md5" // #nosec G501 -- The isolated legacy PDF RC4 compatibility algorithm requires MD5.
	"crypto/rand"
	"crypto/rc4" // #nosec G503 -- This file is explicitly limited to legacy PDF RC4 compatibility.
	"fmt"
	"io"
)

// Advisory bit flag constants that control document activities.
const (
	CnProtectPrint      = 4
	CnProtectModify     = 8
	CnProtectCopy       = 16
	CnProtectAnnotForms = 32
)

// ProtectionAlgorithm names a PDF protection implementation marker.
type ProtectionAlgorithm string

const (
	// ProtectionLegacyRC4 names the legacy RC4-based PDF standard-security
	// handler used by SetLegacyProtection. It is the only document-encryption
	// handler implemented by this package.
	ProtectionLegacyRC4 ProtectionAlgorithm = "legacy-rc4"
)

type protectType struct {
	encrypted     bool
	uValue        []byte
	oValue        []byte
	pValue        int
	padding       []byte
	encryptionKey []byte
	objNum        int
}

func (p *protectType) rc4(n int, buf *[]byte) error {
	if n < 0 || n > 0xFFFFFF {
		return fmt.Errorf("legacy PDF encryption object number out of range: %d", n)
	}
	var key [10]byte
	p.objectKey(n, &key)
	cipher, _ := rc4.NewCipher(key[:]) // #nosec G405 -- Required by the legacy PDF standard-security algorithm.
	cipher.XORKeyStream(*buf, *buf)
	return nil
}

func (p *protectType) objectKey(n int, key *[10]byte) {
	var buf [32]byte
	input := append(buf[:0], p.encryptionKey...)
	input = append(input, byte(n), byte(n>>8), byte(n>>16), 0, 0) // #nosec G115 -- PDF security keys encode the validated 24-bit object number as three bytes.
	sum := md5.Sum(input)                                         // #nosec G401 -- Required by the legacy PDF standard-security algorithm.
	copy(key[:], sum[:10])
}

func oValueGen(userPass, ownerPass []byte) (v []byte) {
	var c *rc4.Cipher
	tmp := md5.Sum(ownerPass)      // #nosec G401 -- Required by the legacy PDF standard-security algorithm.
	c, _ = rc4.NewCipher(tmp[0:5]) // #nosec G405 -- Required by the legacy PDF standard-security algorithm.
	size := len(userPass)
	v = make([]byte, size)
	c.XORKeyStream(v, userPass)
	return
}

func (p *protectType) uValueGen() (v []byte) {
	var c *rc4.Cipher
	c, _ = rc4.NewCipher(p.encryptionKey) // #nosec G405 -- Required by the legacy PDF standard-security algorithm.
	size := len(p.padding)
	v = make([]byte, size)
	c.XORKeyStream(v, p.padding)
	return
}

func (p *protectType) setProtection(privFlag byte, userPassStr, ownerPassStr string) error {
	privFlag = 192 | (privFlag & (CnProtectCopy | CnProtectModify | CnProtectPrint | CnProtectAnnotForms))
	p.padding = []byte{
		0x28, 0xBF, 0x4E, 0x5E, 0x4E, 0x75, 0x8A, 0x41,
		0x64, 0x00, 0x4E, 0x56, 0xFF, 0xFA, 0x01, 0x08,
		0x2E, 0x2E, 0x00, 0xB6, 0xD0, 0x68, 0x3E, 0x80,
		0x2F, 0x0C, 0xA9, 0xFE, 0x64, 0x53, 0x69, 0x7A,
	}
	userPass := []byte(userPassStr)
	var ownerPass []byte
	if ownerPassStr == "" {
		ownerPass = make([]byte, 16)
		if _, err := io.ReadFull(rand.Reader, ownerPass); err != nil {
			return err
		}
	} else {
		ownerPass = []byte(ownerPassStr)
	}
	userPass = append(userPass, p.padding...)[0:32]
	ownerPassBuf := make([]byte, 0, len(ownerPass)+len(p.padding))
	ownerPassBuf = append(ownerPassBuf, ownerPass...)
	ownerPassBuf = append(ownerPassBuf, p.padding...)
	ownerPass = ownerPassBuf[0:32]
	p.encrypted = true
	p.oValue = oValueGen(userPass, ownerPass)
	var buf []byte
	buf = append(buf, userPass...)
	buf = append(buf, p.oValue...)
	buf = append(buf, privFlag, 0xff, 0xff, 0xff)
	sum := md5.Sum(buf) // #nosec G401 -- Required by the legacy PDF standard-security algorithm.
	p.encryptionKey = sum[0:5]
	p.uValue = p.uValueGen()
	p.pValue = -(int(privFlag^255) + 1)
	return nil
}

// SetAESProtection reports that AES-based PDF standard-security encryption is
// intentionally unsupported.
//
// This method exists to make the API boundary explicit: GoPDFKit does not
// half-implement AES document encryption. Use SetLegacyProtection only for the
// legacy RC4 compatibility handler, or use external PDF security tooling when
// AES-based document encryption is required.
func (f *Document) SetAESProtection(actionFlag byte, userPassStr, ownerPassStr string) error {
	_ = actionFlag
	_ = userPassStr
	_ = ownerPassStr
	if f.err != nil {
		return f.err
	}
	err := ErrAESProtectionUnsupported
	f.SetError(err)
	return err
}

// SetLegacyProtection implements the legacy RC4-based PDF standard-security
// handler and reports setup errors directly.
//
// It is provided for compatibility with PDF readers and for advisory viewer
// permissions. Use external encryption, signing, and storage controls when
// modern security is required.
//
// actionFlag is a bit flag that controls various document operations.
// CnProtectPrint allows the document to be printed. CnProtectModify allows a
// document to be modified by a PDF editor. CnProtectCopy allows text and
// images to be copied into the system clipboard. CnProtectAnnotForms allows
// annotations and forms to be added by a PDF editor. These values can be
// combined by ORing them together, for example,
// CnProtectCopy|CnProtectModify. This flag is advisory; not all PDF readers
// implement the constraints that this argument attempts to control.
//
// userPassStr specifies the password that will need to be provided to view the
// contents of the PDF. The permissions specified by actionFlag will apply.
//
// ownerPassStr specifies the password that will need to be provided to gain
// full access to the document regardless of the actionFlag value. An empty
// string for this argument is replaced with a random value, effectively
// preventing owner-level access without the generated password.
func (f *Document) SetLegacyProtection(actionFlag byte, userPassStr, ownerPassStr string) error {
	if f.err != nil {
		return f.err
	}
	if err := f.requireSecurityFeature("legacy RC4 protection", f.securityPolicy.AllowLegacyRC4Protection); err != nil {
		return err
	}
	if err := f.protect.setProtection(actionFlag, userPassStr, ownerPassStr); err != nil {
		f.SetError(err)
		return err
	}
	return nil
}
