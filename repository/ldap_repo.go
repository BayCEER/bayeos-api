package repository

import (
	"errors"
	"fmt"

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
	return &ldapRepo{
		ldapConfig: c,
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

func (r *ldapRepo) Authenticate(name string, password string) (bool, error) {
	l, err := ldap.DialURL(r.ldapConfig.URL)
	if err != nil {
		return false, err
	}
	defer l.Close()
	err = l.Bind(fmt.Sprintf("%s=%s,%s", r.ldapConfig.SignInAttr, ldap.EscapeFilter(name), r.ldapConfig.Base), password)
	if err != nil {
		return false, err
	}
	return true, nil
}

func (r *ldapRepo) GetAttributes(name string) (LDAPAttributes, error) {
	l, err := ldap.DialURL(r.ldapConfig.URL)
	if err != nil {
		return LDAPAttributes{}, err
	}
	defer l.Close()

	searchRequest := ldap.NewSearchRequest(
		r.ldapConfig.Base,
		ldap.ScopeWholeSubtree, ldap.NeverDerefAliases, 0, 0, false,
		fmt.Sprintf("(%s=%s)", r.ldapConfig.SignInAttr, name),
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
