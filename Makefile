.PHONY: ensure only_build only_test all

all: only_build only_test

ensure:
	cd misc/test && dep ensure -v
	# Install aws-iam-authenticator
	# See: https://docs.aws.amazon.com/eks/latest/userguide/install-aws-iam-authenticator.html)
	
	curl -o aws-iam-authenticator https://amazon-eks.s3-us-west-2.amazonaws.com/1.12.7/2019-03-27/bin/linux/amd64/aws-iam-authenticator
	chmod +x ./aws-iam-authenticator
	sudo mv aws-iam-authenticator /usr/local/bin
	
	# Install Pulumi
	curl -L https://get.pulumi.com/ | bash
	export PATH=$HOME/.pulumi/bin:$PATH
	
	# Install kubectl
	curl -LO https://storage.googleapis.com/kubernetes-release/release/$(curl -s https://storage.googleapis.com/kubernetes-release/release/stable.txt)/bin/linux/amd64/kubectl
	chmod +x ./kubectl
	sudo mv kubectl /usr/local/bin

only_build:

only_test:
	go test ./misc/test/... --timeout 4h -v -count=1 -short -parallel 40

# The travis_* targets are entrypoints for CI.
.PHONY: travis_cron travis_push travis_pull_request travis_api
travis_cron: all
travis_push: all
travis_pull_request: all
travis_api: all
