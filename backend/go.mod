module prometheus-dbx-plugin

go 1.22

require github.com/t8y2/dbx/plugins/sdk/go/dbx-plugin-sdk v0.0.0

require gopkg.in/yaml.v3 v3.0.1

replace github.com/t8y2/dbx/plugins/sdk/go/dbx-plugin-sdk => ./sdk
