FROM golang:1.21-alpine AS build
WORKDIR /app
COPY go.mod ./
RUN go mod download
COPY . .
RUN go build -o agility-ring-planner .

FROM alpine:3.19
WORKDIR /app
COPY --from=build /app/agility-ring-planner .
COPY --from=build /app/web ./web
EXPOSE 8080
CMD ["./agility-ring-planner"]
