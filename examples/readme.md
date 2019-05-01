### Examples

#### Running these examples

On Mac OS X, you will need to use Docker to run the `cwlink_bridge example` since the `tap` interface is not supported by default in Mac OS X.

Otherwise, everything can be run as a process. MAC and IP addresses of endpoints are specified in code.

```bash
    go run examples/cwlink_client/main.go
```

Standard AWS environment variables need to be set to read/write to the appropriate AWS  services (Amazon Cloudwatch or AWS Lambda tagS): `AWS_ACCESS_KEY_ID` and `AWS_SECRET_ACCESS_KEY`.

### Examples running in AWS Lambda

There is a [SAM](https://aws.amazon.com/serverless/sam/) template available for deploying a test network to your AWS account.

```bash
    sam package --s3-bucket [your-bucket] > packaged.yaml
    sam deploy --template-file packaged.yaml --stack-name [your-stack] --capabilities CAPABILITY_IAM
```

### Standalone examples

These examples can run anywhere, they just need to be able to read/write to specific AWS services.

##### cwlink_bridge

Amazon Cloudwatch-based network stack designed to be bridged with a local tun or tap interface.

##### cwlink_client

Amazon Cloudwatch-based network stack designed to be run in an unpriviliged environment where interfaces cannot be created (i.e. AWS Lambda)

##### taglink

AWS Lambda Tag-based network stack.

