package GSSAPI

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"github.com/jcmturner/asn1"
	"github.com/jcmturner/gokrb5/asn1tools"
	"github.com/jcmturner/gokrb5/credentials"
	"github.com/jcmturner/gokrb5/crypto"
	"github.com/jcmturner/gokrb5/iana/chksumtype"
	"github.com/jcmturner/gokrb5/messages"
	"github.com/jcmturner/gokrb5/types"
	"math/rand"
)

const (
	TOK_ID_KRB_AP_REQ = "0100"
	TOK_ID_KRB_AP_REP = "0200"
	TOK_ID_KRB_ERROR  = "0300"

	GSS_C_DELEG_FLAG    = 1
	GSS_C_MUTUAL_FLAG   = 2
	GSS_C_REPLAY_FLAG   = 4
	GSS_C_SEQUENCE_FLAG = 8
	GSS_C_CONF_FLAG     = 16
	GSS_C_INTEG_FLAG    = 32
)

type MechToken struct {
	OID      asn1.ObjectIdentifier
	TokID    []byte
	APReq    messages.APReq
	APRep    messages.APRep
	KRBError messages.KRBError
}

func (m *MechToken) Unmarshal(b []byte) error {
	var oid asn1.ObjectIdentifier
	r, err := asn1.UnmarshalWithParams(b, &oid, fmt.Sprintf("application,explicit,tag:%v", 0))
	if err != nil {
		return fmt.Errorf("Error unmarshalling MechToken OID: %v", err)
	}
	m.OID = oid
	m.TokID = r[0:2]
	switch hex.EncodeToString(m.TokID) {
	case TOK_ID_KRB_AP_REQ:
		var a messages.APReq
		err = a.Unmarshal(r[2:])
		if err != nil {
			return fmt.Errorf("Error unmarshalling MechToken AP_REQ: %v", err)
		}
		m.APReq = a
	case TOK_ID_KRB_AP_REP:
		var a messages.APRep
		err = a.Unmarshal(r[2:])
		if err != nil {
			return fmt.Errorf("Error unmarshalling MechToken AP_REP: %v", err)
		}
		m.APRep = a
	case TOK_ID_KRB_ERROR:
		var a messages.KRBError
		err = a.Unmarshal(r[2:])
		if err != nil {
			return fmt.Errorf("Error unmarshalling MechToken KRBError: %v", err)
		}
		m.KRBError = a
	}
	return nil
}

func (m *MechToken) IsAPReq() bool {
	if hex.EncodeToString(m.TokID) == TOK_ID_KRB_AP_REQ {
		return true
	}
	return false
}

func (m *MechToken) IsAPRep() bool {
	if hex.EncodeToString(m.TokID) == TOK_ID_KRB_AP_REP {
		return true
	}
	return false
}

func (m *MechToken) IsKRBError() bool {
	if hex.EncodeToString(m.TokID) == TOK_ID_KRB_ERROR {
		return true
	}
	return false
}

// Create new kerberos AP_REQ MechToken
func NewKRB5APREQMechToken(creds credentials.Credentials, tkt messages.Ticket, sessionKey types.EncryptionKey) ([]byte, error) {
	// Create the header
	b, _ := asn1.Marshal(MechTypeOID_Krb5)
	tb, _ := hex.DecodeString(TOK_ID_KRB_AP_REQ)
	b = append(b, tb...)
	// Add the token
	APReq, err := messages.NewAPReq(
		tkt,
		sessionKey,
		newAuthenticator(creds, sessionKey.KeyType),
	)
	tb, err = APReq.Marshal()
	if err != nil {
		return []byte{}, fmt.Errorf("Could not marshal AP_REQ: %v", err)
	}
	b = append(b, tb...)
	return asn1tools.AddASNAppTag(b, 0), nil
}

// Create new kerberos authenticator for kerberos MechToken
func newAuthenticator(creds credentials.Credentials, keyType int) types.Authenticator {
	//RFC 4121 Section 4.1.1
	auth := types.NewAuthenticator(creds.Realm, creds.CName)
	auth.Cksum = types.Checksum{
		CksumType: chksumtype.GSSAPI,
		Checksum:  newAuthenticatorChksum([]int{GSS_C_INTEG_FLAG, GSS_C_CONF_FLAG}),
	}
	auth.SeqNumber = int(rand.Int31())
	//Generate subkey value
	etype, _ := crypto.GetEtype(keyType)
	sk := make([]byte, etype.GetKeyByteSize())
	rand.Read(sk)
	auth.SubKey = types.EncryptionKey{
		KeyType:  keyType,
		KeyValue: sk,
	}
	return auth
}

// Create new authenticator checksum for kerberos MechToken
func newAuthenticatorChksum(flags []int) []byte {
	a := make([]byte, 24)
	binary.LittleEndian.PutUint32(a[:4], 16)
	for _, i := range flags {
		if i == GSS_C_DELEG_FLAG {
			x := make([]byte, 28-len(a))
			a = append(a, x...)
		}
		f := binary.LittleEndian.Uint32(a[20:24])
		f |= uint32(i)
		binary.LittleEndian.PutUint32(a[20:24], f)
	}
	return a
}

/*
The authenticator checksum field SHALL have the following format:

Octet        Name      Description
-----------------------------------------------------------------
0..3         Lgth    Number of octets in Bnd field;  Represented
			in little-endian order;  Currently contains
			hex value 10 00 00 00 (16).
4..19        Bnd     Channel binding information, as described in
			section 4.1.1.2.
20..23       Flags   Four-octet context-establishment flags in
			little-endian order as described in section
			4.1.1.1.
24..25       DlgOpt  The delegation option identifier (=1) in
			little-endian order [optional].  This field
			and the next two fields are present if and
			only if GSS_C_DELEG_FLAG is set as described
			in section 4.1.1.1.
26..27       Dlgth   The length of the Deleg field in little-endian order [optional].
28..(n-1)    Deleg   A KRB_CRED message (n = Dlgth + 28) [optional].
n..last      Exts    Extensions [optional].
*/
