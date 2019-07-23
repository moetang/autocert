package main

import (
	"encoding/base64"
	"encoding/json"

	"github.com/dgraph-io/badger"
)

func QueryAllDomain() ([]*Domain, error) {
	var queryData [][]byte
	err := db.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()
		prefix := []byte(DomainTablePrefix)
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			item := it.Item()
			err := item.Value(func(v []byte) error {
				queryData = append(queryData, v)
				return nil
			})
			if err != nil {
				return err
			}
		}
		return nil
	})

	if err != nil {
		return nil, err
	}

	result := make([]*Domain, len(queryData))
	for i, v := range queryData {
		domain := new(Domain)
		_ = json.Unmarshal(v, domain)
		result[i] = domain
	}

	return result, nil
}

func QueryAccountByMail(mail string) (*Account, error) {
	var accountData []byte
	err := db.View(func(txn *badger.Txn) error {
		item, err := txn.Get(AccountTable(mail))
		if err != nil {
			return err
		}
		err = item.Value(func(val []byte) error {
			accountData = val
			return nil
		})
		if err != nil {
			return err
		}
		return nil
	})

	if err != nil {
		logline("query account by mail error:", err)
		return nil, err
	}

	accData, err := base64.StdEncoding.DecodeString(string(accountData))
	if err != nil {
		logline("base64 decode error:", err)
		return nil, err
	}

	acc := new(Account)
	err = json.Unmarshal(accData, acc)

	if err != nil {
		logline("json unmarshal error:", err)
		return nil, err
	}
	return acc, nil
}
