package main

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/url"

	"github.com/dgraph-io/badger"
)

var AccountTablePrefix = "account_"

func AccountTable(primaryKey string) []byte {
	return []byte(AccountTablePrefix + primaryKey)
}

func startHttp(laddr string) {
	mux := http.NewServeMux()

	mux.HandleFunc("/register", httpRegisterAccount)
	mux.HandleFunc("/list_account", httpListAccount)

	err := http.ListenAndServe(laddr, mux)
	if err != nil {
		panic(err)
	}
}

func param(key string, q url.Values) *string {
	vs, ok := q[key]
	if !ok || len(vs) == 0 {
		return nil
	}
	v := vs[0]
	return &v
}

func httpListAccount(w http.ResponseWriter, r *http.Request) {
	var queryData [][]byte
	err := db.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()
		prefix := []byte(AccountTablePrefix)
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
		logline("query error:", err)
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("error occurs."))
		return
	}

	result := make([]*Account, len(queryData))
	for i, v := range queryData {
		b, err := base64.StdEncoding.DecodeString(string(v))
		if err != nil {
			logline("decode error:", err)
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte("error occurs."))
			return
		}
		acc := new(Account)
		_ = json.Unmarshal(b, acc)
		// hide private key
		acc.PrivateKeyString = ""
		result[i] = acc
	}

	data, _ := json.Marshal(result)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
	return
}

func httpRegisterAccount(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	// obtain from request
	mailPtr := param("mail", q)
	namePtr := param("name", q)

	if mailPtr == nil || namePtr == nil || len(*mailPtr) == 0 || len(*namePtr) == 0 {
		logline("one of params is empty.")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("error occurs."))
		return
	}

	name := *namePtr
	mail := *mailPtr

	logline("incoming request...", "name:", name, "mail:", mail)

	acc, err := client.Register([]string{mail})
	if err != nil {
		logline("register error:", err)
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("error occurs."))
		return
	}
	acc.AccountName = name
	data, _ := json.Marshal(acc)
	accountData := base64.StdEncoding.EncodeToString(data)
	err = db.Update(func(txn *badger.Txn) error {
		return txn.Set(AccountTable(mail), []byte(accountData))
	})
	if err != nil {
		logline("save error:", err)
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("error occurs."))
		return
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok."))
	return
}
