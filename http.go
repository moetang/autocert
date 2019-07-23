package main

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"time"

	"github.com/dgraph-io/badger"
)

var AccountTablePrefix = "account_"
var DomainTablePrefix = "domain_"

/*
 * //TODO
 * state machine:
 * pending -> challenging
 * challenging -> pending (when token expire/error)
 * challenging -> available (when cert generated)
 * available -> pending (when cert nearly/already expired)
 */
var (
	IssuePending     = "pending"
	IssueChallenging = "challenging"

	IssueAvailable = "available"
)

func AccountTable(primaryKey string) []byte {
	return []byte(AccountTablePrefix + primaryKey)
}

func DomainTable(primaryKey string) []byte {
	return []byte(DomainTablePrefix + primaryKey)
}

func startHttp(laddr string) {
	mux := http.NewServeMux()

	mux.HandleFunc("/register", httpRegisterAccount)
	mux.HandleFunc("/list_account", httpListAccount)

	mux.HandleFunc("/new_issue", httpNewIssue)
	mux.HandleFunc("/list_issue", httpListAllIssue)

	// internal
	mux.HandleFunc("/trigger_job", httpTriggerJob)
	mux.HandleFunc("/delete_issue", httpDeleteIssue)

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

func httpTriggerJob(w http.ResponseWriter, r *http.Request) {
	startJobProcessing()

	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok."))
	return
}

func httpDeleteIssue(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	domainPtr := param("domain", q)
	err := db.Update(func(txn *badger.Txn) error {
		return txn.Delete(DomainTable(*domainPtr))
	})
	if err != nil {
		logline("delete domain error:", err)
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("error occurs."))
		return
	}

	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("submit."))
}

func httpListAllIssue(w http.ResponseWriter, r *http.Request) {
	result, err := QueryAllDomain()

	if err != nil {
		logline("query error:", err)
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("error occurs."))
		return
	}

	data, _ := json.Marshal(result)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
	return
}

func httpNewIssue(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	mailPtr := param("mail", q)
	challengePtr := param("challenge", q)
	domainPtr := param("domain", q)

	if mailPtr == nil || challengePtr == nil || len(*mailPtr) == 0 || len(*challengePtr) == 0 || len(*domainPtr) == 0 || len(*domainPtr) == 0 {
		logline("one of params is empty.")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("error occurs."))
		return
	}

	switch *challengePtr {
	case "dns":
	default:
		logline("challenge is illegal.")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("error occurs."))
		return
	}

	var accountData []byte
	err := db.View(func(txn *badger.Txn) error {
		item, err := txn.Get(AccountTable(*mailPtr))
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
		logline("query error:", err)
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("error occurs."))
		return
	}

	accData, err := base64.StdEncoding.DecodeString(string(accountData))
	if err != nil {
		logline("base64 decode error:", err)
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("error occurs."))
		return
	}
	acc := new(Account)
	err = json.Unmarshal(accData, acc)
	if err != nil {
		logline("json unmarshal error:", err)
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("error occurs."))
		return
	}

	// create issue domain task
	nowTime := time.Now().Format(time.RFC3339Nano)
	domain := &Domain{
		Domain:        *domainPtr,
		AccountMail:   *mailPtr,
		ChallengeType: *challengePtr,
		Status:        IssuePending,

		CreateTime: nowTime,
	}
	domainData, _ := json.Marshal(domain)
	err = db.Update(func(txn *badger.Txn) error {
		// check not exist
		item, err := txn.Get(DomainTable(*domainPtr))
		if err != nil && err != badger.ErrKeyNotFound {
			return err
		}
		if item != nil {
			return errors.New("domain exists")
		}
		return txn.Set(DomainTable(*domainPtr), domainData)
	})
	if err != nil {
		logline("save domain issue job error:", err)
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("error occurs."))
		return
	}

	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("submit."))
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
