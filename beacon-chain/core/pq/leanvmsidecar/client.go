// Copied from github.com/leanEthereum/leanVM bins/leanvm-sidecar/clients/go at 283686575c4e, MIT or Apache 2.0.
// Package leanvmsidecar proves and verifies leanVM aggregates through leanvm-sidecar, over its Unix socket. Claims,
// proofs, signatures and blobs are byte strings in the forms bins/leanvm-sidecar/README.md specifies.
//
// A Client serves one call at a time. After an error other than a *Rejection, close it and dial again. A request
// longer than the sidecar takes (64 MiB) fails so: the sidecar refuses it unread and closes the connection.
package leanvmsidecar

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"time"
)

// The longest response read: a request's claims, at most its 64 MiB, and room for a proof.
const maxResponse = 80 << 20

var errMalformed = errors.New("leanvm-sidecar: a response that does not decode")

// Rejection is the sidecar's refusal of a request (status 1 to 5), which leaves the connection usable.
type Rejection struct {
	Status  byte
	Message string
}

func (r *Rejection) Error() string {
	return fmt.Sprintf("leanvm-sidecar refused the request (status %d): %s", r.Status, r.Message)
}

// Proven is a proof and the claims it proves.
type Proven struct{ Claims, Proof []byte }

// Client is a connection to a sidecar.
type Client struct{ conn net.Conn }

// Dial connects to the sidecar listening on socket, and checks it speaks protocol version 1 for verifier leanvm-v1.
func Dial(socket string) (*Client, error) {
	conn, err := net.Dial("unix", socket)
	if err != nil {
		return nil, err
	}
	hello, err := readFrame(conn)
	if err == nil && (len(hello) < 20 || string(hello[:12]) != "leanvmsc\x01\x00\x00\x00" ||
		!bytes.HasPrefix(hello[20:], appendField(nil, []byte("leanvm-v1")))) {
		err = errors.New("leanvm-sidecar: not a sidecar of protocol version 1 for verifier leanvm-v1")
	}
	if err != nil {
		_ = conn.Close()
		return nil, err
	}
	return &Client{conn}, nil
}

// SetDeadline bounds the next call. Added for Prysm.
func (c *Client) SetDeadline(t time.Time) error { return c.conn.SetDeadline(t) }

// Close closes the connection; the sidecar drops a request it has not started proving.
func (c *Client) Close() error { return c.conn.Close() }

// Aggregate proves the children's claims and the signatures, and commits to the blobs, at the code rate
// 2^-logInvRate (1 to 4). A leanXMSS or leanSPHINCS signature is its pubkey, message and signature, end to end. With
// only nil the proof publishes every claim it covers, else exactly those claims. Children are not verified: verify an
// untrusted one first.
func (c *Client) Aggregate(children []Proven, leanXmss, leanSphincs, blobs [][]byte, only []byte,
	logInvRate byte) (Proven, error) {
	b := binary.LittleEndian.AppendUint64([]byte{1, logInvRate}, uint64(len(children)))
	for _, child := range children {
		b = appendField(appendField(b, child.Claims), child.Proof)
	}
	for _, list := range [][][]byte{leanXmss, leanSphincs, blobs} {
		b = binary.LittleEndian.AppendUint64(b, uint64(len(list)))
		for _, item := range list {
			b = append(b, item...)
		}
	}
	if only == nil {
		b = append(b, 0)
	} else {
		b = appendField(append(b, 1), only)
	}
	r, err := c.call(b)
	if err != nil {
		return Proven{}, err
	}
	claims, r, ok1 := field(r)
	proof, r, ok2 := field(r)
	if !ok1 || !ok2 || len(r) != 0 {
		return Proven{}, errMalformed
	}
	return Proven{claims, proof}, nil
}

// Verify says whether proof proves claims, as the library's AggregateProof::verify does.
func (c *Client) Verify(claims, proof []byte) (bool, error) {
	r, err := c.call(appendField(appendField([]byte{2}, claims), proof))
	if err != nil {
		return false, err
	}
	if len(r) != 1 || r[0] > 1 {
		return false, errMalformed
	}
	return r[0] == 1, nil
}

// call sends a request and returns its successful response, past the status.
func (c *Client) call(request []byte) ([]byte, error) {
	if _, err := c.conn.Write(appendField(nil, request)); err != nil {
		return nil, err
	}
	r, err := readFrame(c.conn)
	if err != nil {
		return nil, err
	}
	if len(r) == 0 {
		return nil, errMalformed
	}
	if r[0] != 0 {
		message, rest, ok := field(r[1:])
		if !ok || len(rest) != 0 {
			return nil, errMalformed
		}
		return nil, &Rejection{r[0], string(message)}
	}
	return r[1:], nil
}

func readFrame(conn io.Reader) ([]byte, error) {
	var prefix [8]byte
	if _, err := io.ReadFull(conn, prefix[:]); err != nil {
		return nil, err
	}
	n := binary.LittleEndian.Uint64(prefix[:])
	if n > maxResponse {
		return nil, errMalformed
	}
	body := make([]byte, n)
	_, err := io.ReadFull(conn, body)
	return body, err
}

// appendField appends v after its u64 length.
func appendField(b, v []byte) []byte {
	return append(binary.LittleEndian.AppendUint64(b, uint64(len(v))), v...)
}

// field splits a length-prefixed field off the front of b.
func field(b []byte) (v, rest []byte, ok bool) {
	if len(b) < 8 || binary.LittleEndian.Uint64(b) > uint64(len(b)-8) {
		return nil, nil, false
	}
	n := 8 + binary.LittleEndian.Uint64(b)
	return b[8:n], b[n:], true
}
