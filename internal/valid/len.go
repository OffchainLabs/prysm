// Package valid holds small, dependency-free helpers for reading values that
// may be nil without first checking them at every call site.
package valid

// Len returns p.Len(), or 0 when p is nil.
//
// P is constrained to pointer types whose method set includes Len() int, which
// covers generated protobuf messages and other pointer-receiver types. The
// constraint is what makes the nil comparison legal without reflection, and it
// means callers no longer depend on each Len implementation tolerating a nil
// receiver. T is inferred from the argument, so calls read valid.Len(x).
func Len[T any, P interface {
	*T
	Len() int
}](p P) int {
	if p == nil {
		return 0
	}
	return p.Len()
}
