package flamingoservice

const ()

type AuthClient struct {
}

type Permission struct {
	Command string
	RoleID  string
	UserID  string
	Allow   bool
}
