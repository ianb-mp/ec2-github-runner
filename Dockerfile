FROM docker.io/alpine
RUN apk update && apk upgrade
ADD build app
ENTRYPOINT /app/ec2-runner