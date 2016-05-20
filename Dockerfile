FROM golang
ADD crossdock/crossdock /
CMD ["/crossdock"]
EXPOSE 8080-8082
