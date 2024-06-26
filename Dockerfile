FROM docker.io/alpine
RUN apk update && apk upgrade
WORKDIR /app
ADD build build
ENTRYPOINT build/ec2-runner