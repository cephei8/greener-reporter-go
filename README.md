# Greener Reporter - Go
Tool to report Go test results to [Greener](https://github.com/cephei8/greener).

## Usage
```console
$ export GREENER_INGRESS_ENDPOINT=http://localhost:8080
$ export GREENER_INGRESS_API_KEY=<API key created in Greener UI>

$ go install github.com/cephei8/greener-reporter-go@latest

$ go test -json ./... 2>&1 | greener-reporter-go -f -
```

Check out [Greener documentation](https://greener.cephei8.dev) or [the main Greener repo](https://github.com/cephei8/greener) for details on how to run the Greener server.

For all the reporter configuration options see `greener-reporter-go --help` or [Plugin Configuration](https://greener.cephei8.dev/plugin-configuration/).

## Contributing
See [CONTRIBUTING.md](./CONTRIBUTING.md).

## License
This project is licensed under the terms of the [Apache License 2.0](./LICENSE).
