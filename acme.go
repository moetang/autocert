package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/base64"
	"fmt"
	"log"

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
}

type AcmeClient struct {
	client acme.Client
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

	fmt.Println("old url:", acc.AccountUrl)
	fmt.Println("new url:", newAcmeAccount.URL)

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
