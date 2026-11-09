package metainfo

func init() {
	if BuildTime == "" {
		BuildTime = "unknown"
	}
}

var Version = "dev-build"
var BuildTime = ""
var ShaVer = "undefined"
