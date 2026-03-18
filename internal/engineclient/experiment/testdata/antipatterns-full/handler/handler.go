// Package handler triggers fat_handler and context_background_in_handler.
package handler

import (
	"context"
	"net/http"
)

// FatHandle is a fat HTTP handler with > 40 statements.
func FatHandle(w http.ResponseWriter, r *http.Request) {
	a := 1
	b := 2
	c := a + b
	d := c * 2
	e := d - 1
	f := e + 3
	g := f * f
	h := g - d
	i := h + a
	j := i * b
	k := j + c
	l := k - e
	m := l + f
	n := m * g
	o := n - h
	p := o + i
	q := p * j
	s := q - k
	u := s + l
	v := u * m
	x := v - n
	y := x + o
	z := y * p
	aa := z - q
	bb := aa + s
	cc := bb * u
	dd := cc - v
	ee := dd + x
	ff := ee * y
	gg := ff - z
	hh := gg + aa
	ii := hh * bb
	jj := ii - cc
	kk := jj + dd
	ll := kk * ee
	mm := ll - ff
	nn := mm + gg
	oo := nn * hh
	pp := oo - ii
	_ = pp
	_ = context.Background()
	w.WriteHeader(http.StatusOK)
}
