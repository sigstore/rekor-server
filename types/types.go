package types

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"strings"

	"github.com/projectrekor/rekor-server/logging"
	"golang.org/x/crypto/openpgp"
)

// RekorEntry is the API request.
type RekorEntry struct {
	Data []byte
	SHA  string
	URL  string
	// Handle other types than GPG
	Signature []byte
	PublicKey []byte
}

func ParseRekorEntry(b []byte) (*RekorEntry, error) {
	var e RekorEntry
	if err := json.Unmarshal(b, &e); err != nil {
		return nil, err
	}
	return &e, nil
}

func (r *RekorEntry) Load() error {
	if r.Data == nil && r.URL == "" {
		return errors.New("one of Contents or ContentsRef must be set")
	}
	publicKey, err := openpgp.ReadArmoredKeyRing(bytes.NewReader(r.PublicKey))
	if err != nil {
		return fmt.Errorf("error reading public key: %s", err)
	}

	var dataReader io.Reader
	if r.URL != "" {
		resp, err := http.DefaultClient.Get(r.URL)
		if err != nil {
			return err
		}

		defer resp.Body.Close()

		b, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return err
		}

		logging.Logger.Info("Contents fetched.")

		// Validate the SHA
		hasher := sha256.New()
		if strings.HasSuffix(r.URL, ".gz") {
			logging.Logger.Info("gzipped content detected")
			gz, err := gzip.NewReader(bytes.NewReader(b))
			if err != nil {
				return err
			}
			io.Copy(hasher, gz)
		} else {
			hasher.Write(b)
		}
		sha := hex.EncodeToString(hasher.Sum(nil))
		if r.SHA != sha {
			return fmt.Errorf("SHA mismatch: %s", r.SHA)
		}

		if strings.HasSuffix(r.URL, ".gz") {
			dataReader, err = gzip.NewReader(dataReader)
			if err != nil {
				return err
			}
		} else {
			dataReader = bytes.NewReader(b)
		}
	} else {
		dataReader = bytes.NewReader(r.Data)
	}

	verifyFn := openpgp.CheckDetachedSignature
	if strings.Contains(string(r.Signature), "-----BEGIN PGP") {
		logging.Logger.Info("Armored signature detected")
		verifyFn = openpgp.CheckArmoredDetachedSignature
	} else {
		logging.Logger.Info("Binary signature detected")
	}

	if _, err := verifyFn(publicKey, dataReader, bytes.NewReader(r.Signature)); err != nil {
		return err
	}

	if r.SHA == "" {
		r.SHA = hex.EncodeToString(sha256.New().Sum(r.Data))
	}

	return nil
}

func (r *RekorEntry) Leaf() RekorLeaf {
	return RekorLeaf{
		SHA:       r.SHA,
		Signature: r.Signature,
		PublicKey: r.PublicKey,
	}
}

func (r *RekorEntry) MarshalledLeaf() ([]byte, error) {
	return json.Marshal(r.Leaf())
}

// RekorLeaf is the type we store in the log.
type RekorLeaf struct {
	SHA       string
	Signature []byte
	PublicKey []byte
}
