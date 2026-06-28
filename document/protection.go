// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package document

import (
	"crypto/md5"
	"crypto/rand"
	"crypto/rc4"
	"io"
)

// Advisory bit flag constants that control document activities.
const (
	CnProtectPrint      = 4
	CnProtectModify     = 8
	CnProtectCopy       = 16
	CnProtectAnnotForms = 32
)

type protectType struct {
	encrypted     bool
	uValue        []byte
	oValue        []byte
	pValue        int
	padding       []byte
	encryptionKey []byte
	objNum        int
	rc4cipher     *rc4.Cipher
	rc4n          uint32 // Object number associated with the RC4 cipher.
}

func (p *protectType) rc4(n uint32, buf *[]byte) {
	if p.rc4cipher == nil || p.rc4n != n {
		var key [10]byte
		p.objectKey(n, &key)
		p.rc4cipher, _ = rc4.NewCipher(key[:])
		p.rc4n = n
	}
	p.rc4cipher.XORKeyStream(*buf, *buf)
}

func (p *protectType) objectKey(n uint32, key *[10]byte) {
	var buf [32]byte
	input := append(buf[:0], p.encryptionKey...)
	input = append(input, byte(n), byte(n>>8), byte(n>>16), 0, 0)
	sum := md5.Sum(input)
	copy(key[:], sum[:10])
}

func oValueGen(userPass, ownerPass []byte) (v []byte) {
	var c *rc4.Cipher
	tmp := md5.Sum(ownerPass)
	c, _ = rc4.NewCipher(tmp[0:5])
	size := len(userPass)
	v = make([]byte, size)
	c.XORKeyStream(v, userPass)
	return
}

func (p *protectType) uValueGen() (v []byte) {
	var c *rc4.Cipher
	c, _ = rc4.NewCipher(p.encryptionKey)
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
	sum := md5.Sum(buf)
	p.encryptionKey = sum[0:5]
	p.uValue = p.uValueGen()
	p.pValue = -(int(privFlag^255) + 1)
	return nil
}

// SetProtection applies the legacy RC4-based PDF standard-security handler as
// a compatibility wrapper.
//
// Deprecated: use SetLegacyProtection so new code names the compatibility and
// advisory-security behavior explicitly. This is not modern document
// encryption, secure storage, or a DRM guarantee.
func (f *Document) SetProtection(actionFlag byte, userPassStr, ownerPassStr string) {
	_ = f.SetLegacyProtection(actionFlag, userPassStr, ownerPassStr)
}

// SetProtectionError applies legacy PDF standard-security protection and
// reports setup errors directly.
//
// Deprecated: use SetLegacyProtection.
func (f *Document) SetProtectionError(actionFlag byte, userPassStr, ownerPassStr string) error {
	return f.SetLegacyProtection(actionFlag, userPassStr, ownerPassStr)
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
