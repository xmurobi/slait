module github.com/alpacahq/slait

require (
	code.cloudfoundry.org/bytefmt v0.0.0-20180906201452-2aa6f33b730c
	github.com/eapache/channels v1.1.0
	github.com/eapache/queue v1.1.0 // indirect
	github.com/fatih/structs v1.1.0 // indirect
	github.com/golang/glog v0.0.0-20160126235308-23def4e6c14b
	github.com/gopherjs/gopherjs v0.0.0-20181103185306-d547d1d9531e // indirect
	github.com/gorilla/websocket v1.4.0
	github.com/jasonlvhit/gocron v0.0.0-20180312192515-54194c9749d4
	github.com/juju/errors v0.0.0-20181012004132-a4583d0a56ea // indirect
	github.com/kataras/iris v0.0.0-20181106020650-c20bc3bceef1
	github.com/kr/pretty v0.1.0 // indirect
	github.com/pkg/errors v0.8.0
	github.com/xeipuuv/gojsonschema v0.0.0-20181016150526-f3a9dae5b194 // indirect
	golang.org/x/crypto v0.0.0-20181106152344-bfa7d42eb568 // indirect
	golang.org/x/net v0.0.0-20181102091132-c10e9556a7bc
	gopkg.in/check.v1 v1.0.0-20180628173108-788fd7840127
	gopkg.in/yaml.v2 v2.2.1
)

replace (
	golang.org/x/crypto v0.0.0-20180927165925-5295e8364332 => github.com/golang/crypto v0.0.0-20180927165925-5295e8364332
	golang.org/x/crypto v0.0.0-20181106152344-bfa7d42eb568 => github.com/golang/crypto v0.0.0-20181106152344-bfa7d42eb568
	golang.org/x/net v0.0.0-20180724234803-3673e40ba225 => github.com/golang/net v0.0.0-20180724234803-3673e40ba225
	golang.org/x/net v0.0.0-20180906233101-161cd47e91fd => github.com/golang/net v0.0.0-20180906233101-161cd47e91fd
	golang.org/x/net v0.0.0-20181023162649-9b4f9f5ad519 => github.com/golang/net v0.0.0-20181023162649-9b4f9f5ad519
	golang.org/x/net v0.0.0-20181102091132-c10e9556a7bc => github.com/golang/net v0.0.0-20181102091132-c10e9556a7bc
	golang.org/x/sync v0.0.0-20180314180146-1d60e4601c6f => github.com/golang/sync v0.0.0-20180314180146-1d60e4601c6f
	golang.org/x/sys v0.0.0-20180909124046-d0be0721c37e => github.com/golang/sys v0.0.0-20180909124046-d0be0721c37e
	golang.org/x/sys v0.0.0-20180928133829-e4b3c5e90611 => github.com/golang/sys v0.0.0-20180928133829-e4b3c5e90611
	golang.org/x/sys v0.0.0-20181024145615-5cd93ef61a7c => github.com/golang/sys v0.0.0-20181024145615-5cd93ef61a7c
	golang.org/x/text v0.3.0 => github.com/golang/text v0.3.0
	google.golang.org/appengine v1.3.0 => github.com/golang/appengine v1.3.0
)
