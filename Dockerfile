# Based on https://www.digitalocean.com/community/tutorials/how-to-deploy-a-go-web-application-with-docker-and-nginx-on-ubuntu-22-04

FROM golang:alpine AS build
RUN apk --no-cache add gcc g++ make git
WORKDIR /go/src/app
# Force check for changes so know when to rebuild
ADD "https://api.github.com/repos/RogerTheRabbit/todo-app-backend-golang/commits?per_page=1" latest_commit
RUN rm ./latest_commit
RUN git clone https://github.com/RogerTheRabbit/todo-app-backend-golang.git /go/src/app --depth 1
RUN go mod tidy
RUN GOOS=linux go build -ldflags="-s -w" -o ./bin/web-app ./todo.go

FROM alpine:3.17
RUN apk --no-cache add ca-certificates
WORKDIR /usr/bin
COPY --from=build /go/src/app/bin /go/bin
EXPOSE 8080
ENTRYPOINT /go/bin/web-app
