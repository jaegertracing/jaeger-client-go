FROM golang
ADD crossdock-main /
CMD ["/crossdock-main"]
EXPOSE 8080-8082
