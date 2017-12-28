package socks_go

import "io"

type fakeTransport struct {
	input  chan byte
	output []byte
}

func (tr *fakeTransport) Read(p []byte) (n int, err error) {
	size := len(p)
	if size == 0 {
		return
	}

	for b := range tr.input {
		p[n] = b
		n++
		size--
		if size == 0 {
			return
		}
	}

	if n < size {
		err = io.EOF
	}
	return
}

func (tr *fakeTransport) Write(p []byte) (n int, err error) {
	tr.output = append(tr.output, p...)
	n = len(p)
	return
}

func (tr *fakeTransport) Send(p []byte) {
	for _, b := range p {
		tr.input <- b
	}
}

func newFakeTransport() fakeTransport {
	return fakeTransport{make(chan byte, 1024), []byte{}}
}
