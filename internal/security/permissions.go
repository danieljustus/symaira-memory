package security

// Role represents a permission level for agent profiles.
type Role string

const (
	RoleRead      Role = "read"
	RoleReadWrite Role = "readwrite"
	RoleAdmin     Role = "admin"
)

// CanWrite returns true if the role permits write operations (memory_set, delete, sync/apply).
func (r Role) CanWrite() bool {
	return r == RoleReadWrite || r == RoleAdmin
}

// CanAdmin returns true if the role permits administrative operations.
func (r Role) CanAdmin() bool {
	return r == RoleAdmin
}

// ParseRole converts a string to a Role, defaulting to RoleReadWrite for unknown values.
func ParseRole(s string) Role {
	switch Role(s) {
	case RoleRead, RoleReadWrite, RoleAdmin:
		return Role(s)
	default:
		return RoleReadWrite
	}
}
