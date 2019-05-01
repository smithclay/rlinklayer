### Custom AWS Lambda Runtime

#### Running

```bash
    $ GOOS=linux GOARCH=amd64 go build -o bootstrap ./bootstrap.go
    $ sam local invoke -e event.json
```

#### Deploying

```bash
    $ sam package \
        --template-file template.yaml \
        --output-template-file packaged.yaml \
        --s3-bucket rlinklayer-test-bckt
```