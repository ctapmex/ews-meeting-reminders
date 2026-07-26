# Build static ews-reminders binary (no runtime image).
# Extract:
#   docker build --target export --output type=local,dest=./bin .
# Or: ./docker-build.sh
FROM golang:bookworm AS build
ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_TIME=unknown
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN make ews-reminders OUT_DIR=/out VERSION=${VERSION} COMMIT=${COMMIT} BUILD_TIME=${BUILD_TIME}

FROM scratch AS export
COPY --from=build /out/ews-reminders /ews-reminders
