package types

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"golang.org/x/crypto/openpgp/armor"
	"golang.org/x/crypto/openpgp/packet"

	"golang.org/x/crypto/openpgp"
	"golang.org/x/sync/errgroup"
)

// RekorEntry is the API request.
type RekorEntry struct {
	Data      []byte
	URL       string
	RekorLeaf `json:"-"`
}

// RekorLeaf is the type we store in the log.
type RekorLeaf struct {
	SHA       string
	Signature []byte
	PublicKey []byte
	PubKeyEnt openpgp.EntityList `json:"-"`
	ArmorSig  bool               `json:"-"`
}

func ParseRekorLeaf(r io.Reader) (*RekorLeaf, error) {
	var l RekorLeaf
	dec := json.NewDecoder(r)
	if err := dec.Decode(&l); err != nil && err != io.EOF {
		return nil, err
	}

	// validate fields
	if l.SHA != "" {
		if _, err := hex.DecodeString(l.SHA); err != nil || len(l.SHA) != 64 {
			return nil, fmt.Errorf("Invalid SHA hash provided")
		}
	}

	// check if this is an actual signature
	var sigReader io.Reader
	sigByteReader := bytes.NewReader(l.Signature)
	sigBlock, err := armor.Decode(sigByteReader)
	if err == nil {
		l.ArmorSig = true
		if sigBlock.Type != openpgp.SignatureType {
			return nil, fmt.Errorf("Invalid signature provided")
		}
		sigReader = sigBlock.Body
	} else {
		l.ArmorSig = false
		if _, err := sigByteReader.Seek(0, io.SeekStart); err != nil {
			return nil, err
		}
		sigReader = sigByteReader
	}
	sigPktReader := packet.NewReader(sigReader)
	sigPkt, err := sigPktReader.Next()
	if err != nil {
		return nil, fmt.Errorf("Invalid signature provided")
	}
	if _, ok := sigPkt.(*packet.Signature); ok != true {
		if _, ok := sigPkt.(*packet.SignatureV3); ok != true {
			return nil, fmt.Errorf("Invalid signature provided")
		}
	}

	// check if this is an actual public key
	var keyReader io.Reader
	keyByteReader := bytes.NewReader(l.PublicKey)
	keyBlock, err := armor.Decode(keyByteReader)
	if err == nil {
		if keyBlock.Type != openpgp.PublicKeyType {
			return nil, fmt.Errorf("Invalid public key provided")
		}
		keyReader = keyBlock.Body
	} else {
		if _, err := keyByteReader.Seek(0, io.SeekStart); err != nil {
			return nil, err
		}
		keyReader = keyByteReader
	}

	pubKey, err := openpgp.ReadKeyRing(keyReader)
	if err != nil {
		return nil, fmt.Errorf("Invalid public key provided")
	}
	l.PubKeyEnt = pubKey

	return &l, nil
}

func ParseRekorEntry(r io.Reader, leaf RekorLeaf) (*RekorEntry, error) {
	var e RekorEntry
	dec := json.NewDecoder(r)
	if err := dec.Decode(&e); err != nil && err != io.EOF {
		return nil, err
	}
	//decode above should not have included the previously parsed & validated leaf, so copy it in
	e.RekorLeaf = leaf

	if e.Data == nil && e.URL == "" {
		return nil, errors.New("one of Contents or ContentsRef must be set")
	}

	if e.URL != "" && e.SHA == "" {
		return nil, errors.New("SHA hash must be specified if URL is set")
	}

	return &e, nil
}

func (r *RekorEntry) Load(ctx context.Context) error {

	hashR, hashW := io.Pipe()
	pgpR, pgpW := io.Pipe()

	var dataReader io.Reader
	if r.URL != "" {
		//TODO: set timeout here, SSL settings?
		resp, err := http.DefaultClient.Get(r.URL)
		if err != nil {
			return err
		}
		defer resp.Body.Close()

		// read first 512 bytes to determine if content is gzip compressed
		bufReader := bufio.NewReaderSize(resp.Body, 512)
		ctBuf, err := bufReader.Peek(512)
		if err != nil && err != bufio.ErrBufferFull && err != io.EOF {
			return err
		}

		if "application/x+gzip" == http.DetectContentType(ctBuf) {
			dataReader, _ = gzip.NewReader(io.MultiReader(bufReader, resp.Body))
		} else {
			dataReader = io.MultiReader(bufReader, resp.Body)
		}
	} else {
		dataReader = bytes.NewReader(r.Data)
	}

	g, ctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		defer hashW.Close()
		defer pgpW.Close()

		/* #nosec G110 */
		if _, err := io.Copy(io.MultiWriter(hashW, pgpW), dataReader); err != nil {
			return err
		}
		return nil
	})

	hashResult := make(chan string)

	g.Go(func() error {
		defer hashR.Close()
		defer close(hashResult)

		hasher := sha256.New()

		if _, err := io.Copy(hasher, hashR); err != nil {
			return err
		}

		computedSHA := hex.EncodeToString(hasher.Sum(nil))
		if r.SHA != "" && computedSHA != r.SHA {
			return fmt.Errorf("SHA mismatch: %s != %s", computedSHA, r.SHA)
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case hashResult <- computedSHA:
			return nil
		}
	})

	g.Go(func() error {
		defer pgpR.Close()

		verifyFn := openpgp.CheckDetachedSignature
		if r.ArmorSig == true {
			verifyFn = openpgp.CheckArmoredDetachedSignature
		}

		if _, err := verifyFn(r.PubKeyEnt, pgpR, bytes.NewReader(r.Signature)); err != nil {
			return err
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			return nil
		}
	})

	computedSHA := <-hashResult

	if err := g.Wait(); err != nil {
		return err
	}

	// if we get here, all goroutines succeeded without error
	if r.SHA == "" {
		r.SHA = computedSHA
	}

	return nil
}
