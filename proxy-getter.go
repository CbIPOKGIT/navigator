package navigator

type ProxyGetter interface {

	// Get proxy.
	//
	// Returns proxy as string and error if has
	GetProxy() (string, error)
}
