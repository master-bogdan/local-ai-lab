package buildinfo

var (
	Version = "dev"
	Commit  = "unknown"
)

func IsDevelopment() bool {
	return Version == "dev"
}
