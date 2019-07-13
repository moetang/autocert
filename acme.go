package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/base64"
	"log"

	"github.com/eggsampler/acme"
)

type Account struct {
	PrivateKeyString string

	AccountUrl string

	MailList []string
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
