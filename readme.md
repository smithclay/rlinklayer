## Richard Linklayer (rlinklayer)

experimental userspace network overlay designed for serverless functions that tunnels through Amazon Cloudwatch Logs or AWS Lambda tags that uses Google's [netstack](https://github.com/google/netstack) library.

You could use this library to:

* Run non-privilged servers inside of AWS Lambda and access them over TCP/IP (slowly)
* Establish a (slow) TCP or UDP connection between function(s)
* Bridge a network running inside of AWS Lambda functions with a normal network.

The current state of this project is proof-of-concept/experimental. This isn't meant for anything production.

### running on AWS Lambda

Currently, this is only supported with a custom runtime. See `examples/template.yaml` for a sample function that runs an HTTP server in node.js.

A publicly-deployable AWS Lambda Layer is also provided for running with your own functions.

### running locally on Mac OS X

Special permissions are needed in Docker to create a tun or tap network interface. Because AWS Services are used as a link layer transport, AWS credentials are needed:

First, build the docker image:

```sh
    docker build -t smithclay/rlinklayer .
```

Next, run the container with AWS credentials that can write and read to Amazon Cloudwatch Logs.

The `start-server.sh` script automatically creates a tun or tap network interface in the docker container, if one does not exist.

```sh
    docker run --env AWS_ACCESS_KEY_ID=<<access key id>> env AWS_SECRET_ACCESS_KEY=<access_key>>--name richard-linklayer --privileged smithclay/rlinklayer ./start-server.sh
```

Then, run do networking stuff that interacts with an IP address that's a running function.

```sh
    docker exec -ti richard-linklayer ping 192.168.1.21
```

### examples

Examples are in the `examples` directory.

