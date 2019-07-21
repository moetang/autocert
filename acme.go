package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"io/ioutil"
	"log"
	"strings"
	"time"

	"github.com/eggsampler/acme"
)

type Account struct {
	PrivateKeyString string

	AccountUrl string

	AccountName string
	MailList    []string

	acmeAccount *acme.Account
}

type Domain struct {
	Domain        string
	AccountMail   string
	ChallengeType string
	Status        string

	CreateTime string
	IssueTime  string

	ChallengeData string
	OrderData     string
}

type AcmeClient struct {
	client acme.Client
}

// copy from acme client
// in order to store data of non-json fields of the origin
type Challenge struct {
	Type      string       `json:"type"`
	URL       string       `json:"url"`
	Status    string       `json:"status"`
	Validated string       `json:"validated"`
	Error     acme.Problem `json:"error"`

	// Based on the challenge used
	Token            string `json:"token"`
	KeyAuthorization string `json:"keyAuthorization"`

	// Authorization url provided by the rel="up" Link http header
	AuthorizationURL string `json:"authorizationURL"`
}

func challengeConvertLocal(chal acme.Challenge) Challenge {
	return Challenge{
		Type:             chal.Type,
		URL:              chal.URL,
		Status:           chal.Status,
		Validated:        chal.Validated,
		Error:            chal.Error,
		Token:            chal.Token,
		KeyAuthorization: chal.KeyAuthorization,
		AuthorizationURL: chal.AuthorizationURL,
	}
}

func challengeConvertOrigin(chal acme.Challenge) acme.Challenge {
	return acme.Challenge{
		Type:             chal.Type,
		URL:              chal.URL,
		Status:           chal.Status,
		Validated:        chal.Validated,
		Error:            chal.Error,
		Token:            chal.Token,
		KeyAuthorization: chal.KeyAuthorization,
		AuthorizationURL: chal.AuthorizationURL,
	}
}

func newAcmeClient(production bool) *AcmeClient {
	var url string
	if production {
		url = acme.LetsEncryptProduction
	} else {
		url = acme.LetsEncryptStaging
	}
	client, err := acme.NewClient(url)
	if err != nil {
		log.Fatalf("Error connecting to acme directory: %v", err)
	}
	client.PollTimeout = 5 * time.Second
	return &AcmeClient{
		client: client,
	}
}

func (this *AcmeClient) LoadAccount(acc *Account) (*Account, error) {
	privData, err := base64.StdEncoding.DecodeString(acc.PrivateKeyString)
	if err != nil {
		return nil, err
	}
	privKey, err := x509.ParseECPrivateKey(privData)
	if err != nil {
		return nil, err
	}

	acmeAccount := acme.Account{
		PrivateKey: privKey,
		URL:        acc.AccountUrl,
	}
	contacts := make([]string, len(acc.MailList))
	for i, v := range acc.MailList {
		contacts[i] = "mailto:" + v
	}
	newAcmeAccount, err := this.client.UpdateAccount(acmeAccount, true, contacts...)
	if err != nil {
		return nil, err
	}

	acc.acmeAccount = &newAcmeAccount
	acc.AccountUrl = newAcmeAccount.URL

	return acc, nil
}

func (this *AcmeClient) Register(mailList []string) (*Account, error) {
	privKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	contacts := make([]string, len(mailList))
	for i, v := range mailList {
		contacts[i] = "mailto:" + v
	}

	acc, err := this.client.NewAccount(privKey, false, true, contacts...)
	if err != nil {
		return nil, err
	}

	privKeyData, err := x509.MarshalECPrivateKey(privKey)
	if err != nil {
		return nil, err
	}

	account := new(Account)
	account.PrivateKeyString = base64.StdEncoding.EncodeToString(privKeyData)
	account.AccountUrl = acc.URL
	account.MailList = mailList

	return account, nil
}

func (this *AcmeClient) AcquireChallenging(acc *Account, domain *Domain) (orderData, chaldata []byte, token string, err error) {
	// do acme
	ids := []acme.Identifier{
		{
			Type:  "dns",
			Value: domain.Domain,
		},
	}
	order, err := this.client.NewOrder(*acc.acmeAccount, ids)
	if err != nil {
		logline("new acme order error. err=[", err, "] domain:", domain.Domain, " mail:", domain.AccountMail)
		return nil, nil, "", err
	}
	for _, authUrl := range order.Authorizations {
		auth, err := this.client.FetchAuthorization(*acc.acmeAccount, authUrl)
		if err != nil {
			logline("Error fetching authorization url ", authUrl, ":", err)
			return nil, nil, "", err
		}
		// only use dns challenge
		chal, ok := auth.ChallengeMap[acme.ChallengeTypeDNS01]
		if !ok {
			logline("Unable to find dns challenge for auth ", domain.Domain)
		}

		j, _ := json.Marshal(challengeConvertLocal(chal))
		o, _ := json.Marshal(order)

		// current only 1 order
		return o, j, chal.Token, nil
	}

	return nil, nil, "", errors.New("no authorization")
}

func (this *AcmeClient) UpdateChallenge(acc *Account, domain *Domain) (privKeyData, certData []byte, err error) {
	chalLocal := acme.Challenge{}
	err = json.Unmarshal([]byte(domain.ChallengeData), &chalLocal)
	if err != nil {
		logline("update challenge unmarshal chal failed:", err)
		return nil, nil, err
	}
	chal := challengeConvertOrigin(chalLocal)

	newChal, err := this.client.UpdateChallenge(*acc.acmeAccount, chal)
	if err != nil {
		logline("acme update challenge error:", err)
		return nil, nil, err
	}
	//TODO need remove
	logline("new chal is:", newChal)

	// generate ecdsa certificate
	privKey, privKeyData, csr, err := GenerateECDSA256Certificate(domain.Domain, []string{})
	var _ = privKey
	if err != nil {
		logline("generate ecdsa 256 certificate error:", err)
		return nil, nil, err
	}

	order := acme.Order{}
	err = json.Unmarshal([]byte(domain.OrderData), &order)
	if err != nil {
		logline("update challenge unmarshal order failed:", err)
		return nil, nil, err
	}

	order, err = this.client.FinalizeOrder(*acc.acmeAccount, order, csr)
	if err != nil {
		logline("finalize order failed:", err)
		return nil, nil, err
	}
	certs, err := this.client.FetchCertificates(*acc.acmeAccount, order.Certificate)
	if err != nil {
		logline("fetch certificates failed:", err)
		return nil, nil, err
	}

	//TODO need remove
	logline("cert generated:", certs)
	// current only 1 cert
	certPemData := strings.TrimSpace(string(pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: certs[0].Raw,
	})))
	return privKeyData, []byte(certPemData), nil
}

func GenerateECDSA256Certificate(domainName string, domainList []string) (privKey *ecdsa.PrivateKey, privKeyData []byte, csr *x509.CertificateRequest, err error) {
	certKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		logline("generate ecdsa 256 private key error:", err)
		return nil, nil, nil, err
	}
	// encode the new ec private key
	certKeyEnc, err := x509.MarshalECPrivateKey(certKey)
	if err != nil {
		logline("encoding ecdsa 256 private key error:", err)
		return nil, nil, nil, err
	}

	tpl := &x509.CertificateRequest{
		SignatureAlgorithm: x509.ECDSAWithSHA256,
		PublicKeyAlgorithm: x509.ECDSA,
		PublicKey:          certKey.Public(),
		Subject:            pkix.Name{CommonName: domainName},
		DNSNames:           domainList,
	}
	csrDer, err := x509.CreateCertificateRequest(rand.Reader, tpl, certKey)
	if err != nil {
		logline("creating certificate error:", err)
		return nil, nil, nil, err
	}
	csr, err = x509.ParseCertificateRequest(csrDer)
	if err != nil {
		logline("parsing certificate error:", err)
		return nil, nil, nil, err
	}
	return certKey, certKeyEnc, csr, nil
}

func WritePemPrivateKeyFile(f string, key []byte) error {
	if err := ioutil.WriteFile(f, pem.EncodeToMemory(&pem.Block{
		Type:  "EC PRIVATE KEY",
		Bytes: key,
	}), 0600); err != nil {
		return err
	}
	return nil
}

func WritePemCertFile(f string, cert []byte) error {
	if err := ioutil.WriteFile(f, cert, 0600); err != nil {
		return err
	}
	return nil
}
