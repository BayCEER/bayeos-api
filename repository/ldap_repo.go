package repository

import (
	"crypto/tls"
	"errors"
	"fmt"
	"strings"

	"github.com/go-ldap/ldap/v3"
)

type LDAPAttributes struct {
	DisplayName string
	Email       string
}

type LdapRepo interface {
	Authenticate(name string, password string) (bool, error)
	GetAttributes(name string) (LDAPAttributes, error)
	Enabled() bool
}

type ldapRepo struct {
	ldapConfig LdapConfig
	tlsConfig  *tls.Config
}

type LdapConfig struct {
	Enabled         bool
	URL             string
	Base            string
	SignInAttr      string
	DisplayNameAttr string
	EmailAttr       string
}

func NewLdapRepoFromConfig(c LdapConfig) LdapRepo {

	var dynamicCiphers []uint16

	// Add secure cipher suites to the list of dynamic ciphers
	for _, cipher := range tls.CipherSuites() {
		dynamicCiphers = append(dynamicCiphers, cipher.ID)
	}

	// Add insecure cipher suites to the list of dynamic ciphers
	for _, cipher := range tls.InsecureCipherSuites() {
		dynamicCiphers = append(dynamicCiphers, cipher.ID)
	}

	return &ldapRepo{
		ldapConfig: c,
		tlsConfig: &tls.Config{
			MinVersion:   tls.VersionTLS12,
			MaxVersion:   tls.VersionTLS13,
			CipherSuites: dynamicCiphers,
		},
	}

}

func NewDisabledLdapRepo() LdapRepo {
	return &ldapRepo{
		ldapConfig: LdapConfig{
			Enabled:         false,
			URL:             "",
			Base:            "",
			SignInAttr:      "",
			DisplayNameAttr: "",
			EmailAttr:       "",
		},
	}
}

func (r *ldapRepo) dialURL() (*ldap.Conn, error) {

	l, err := ldap.DialURL(r.ldapConfig.URL, ldap.DialWithTLSConfig(r.tlsConfig))
	if err != nil {
		return nil, err
	}
	return l, nil

}

func (r *ldapRepo) Authenticate(name string, password string) (bool, error) {
	l, err := r.dialURL()
	if err != nil {
		return false, err
	}
	defer l.Close()
	err = l.Bind(fmt.Sprintf("%s=%s,%s", r.ldapConfig.SignInAttr, escapeDNAttrValue(name), r.ldapConfig.Base), password)
	if err != nil {
		return false, err
	}
	return true, nil
}

func (r *ldapRepo) GetAttributes(name string) (LDAPAttributes, error) {
	l, err := r.dialURL()
	if err != nil {
		return LDAPAttributes{}, err
	}
	defer l.Close()

	searchRequest := ldap.NewSearchRequest(
		r.ldapConfig.Base,
		ldap.ScopeWholeSubtree, ldap.NeverDerefAliases, 0, 0, false,
		fmt.Sprintf("(%s=%s)", r.ldapConfig.SignInAttr, ldap.EscapeFilter(name)),
		[]string{r.ldapConfig.DisplayNameAttr, r.ldapConfig.EmailAttr},
		nil,
	)
	sr, err := l.Search(searchRequest)
	if err != nil {
		return LDAPAttributes{}, err
	}
	if len(sr.Entries) == 1 {
		return LDAPAttributes{
			DisplayName: sr.Entries[0].GetAttributeValue(r.ldapConfig.DisplayNameAttr),
			Email:       sr.Entries[0].GetAttributeValue(r.ldapConfig.EmailAttr)}, nil
	} else {
		return LDAPAttributes{}, errors.New("user attributes not found")
	}

}

func (r *ldapRepo) Enabled() bool {
	return r.ldapConfig.Enabled
}

func escapeDNAttrValue(v string) string {
	if v == "" {
		return v
	}
	var b strings.Builder
	for i := 0; i < len(v); i++ {
		c := v[i]
		switch c {
		case ',', '+', '"', '\\', '<', '>', ';':
			b.WriteByte('\\')
			b.WriteByte(c)
		case '#':
			if i == 0 {
				b.WriteString("\\#")
			} else {
				b.WriteByte(c)
			}
		case ' ':
			if i == 0 || i == len(v)-1 {
				b.WriteString("\\ ")
			} else {
				b.WriteByte(c)
			}
		default:
			b.WriteByte(c)
		}
	}
	return b.String()
}
