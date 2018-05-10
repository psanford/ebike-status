credfile = $(HOME)/.aws_creds_pms

ebike-status: main.go
	go build .

.PHONY: upload
upload: ebike-status
	zip ebike-status.zip ebike-status
	AWS_ACCESS_KEY_ID=$(shell sed -nr 's/AWSAccessKeyId=(.*)/\1/p' $(credfile)) \
	AWS_SECRET_ACCESS_KEY=$(shell sed -nr 's/AWSSecretKey=(.*)/\1/p' $(credfile)) \
	aws lambda update-function-code --function-name ebike-status --zip-file fileb://ebike-status.zip
	rm ebike-status.zip
	rm ebike-status
