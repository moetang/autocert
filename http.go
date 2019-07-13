package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/dgraph-io/badger"
	"net/http"
)

var AccountTablePrefix = "account_"

func AccountTable(primaryKey string) []byte {
	return []byte(AccountTablePrefix + primaryKey)
}

func startHttp(laddr string) {
	mux := http.NewServeMux()

	mux.HandleFunc("/register", httpRegisterAccount)

	err := http.ListenAndServe(laddr, mux)
	if err != nil {
		panic(err)
	}
}

func httpRegisterAccount(w http.ResponseWriter, r *http.Request) {
	fmt.Println("incoming request...")

	mail := "meidomx2@outlook.com"

	acc, err := client.Register([]string{mail})
	if err != nil {
		fmt.Println("register error:", err)
		w.WriteHeader(http.StatusOK)
		return
	}
	data, _ := json.Marshal(acc)
	accountData := base64.StdEncoding.EncodeToString(data)
	fmt.Println(accountData)
	err = db.Update(func(txn *badger.Txn) error {
		return txn.Set(AccountTable(mail), []byte(accountData))
	})
	if err != nil {
		fmt.Println("save error:", err)
		w.WriteHeader(http.StatusOK)
		return
	}
	w.WriteHeader(http.StatusOK)
	return
}
