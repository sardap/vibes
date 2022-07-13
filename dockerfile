# React frotnend
FROM node:8 as builder
WORKDIR /app
# Seprated for caching
COPY ./frontend/package.json .
COPY ./frontend/package-lock.json .
RUN npm install .
COPY ./frontend/src src
COPY ./frontend/public public
RUN npm run build

# Backend
FROM rust:1.62.0-slim-buster

ENV DEBIAN_FRONTEND=noninteractive

RUN apt-get update && apt-get install -y ffmpeg python3 python3-pip pkg-config libssl-dev

RUN mkdir app

COPY backend/audio_gen/requirements.txt app/audio_gen/requirements.txt

RUN pip3 install -r /app/audio_gen/requirements.txt

COPY backend/Cargo.lock app/Cargo.lock
COPY backend/Cargo.toml app/Cargo.toml
COPY backend/src app/src

WORKDIR /app
RUN cargo build --release

COPY backend/audio_gen/startup.py /app/audio_gen/startup.py

COPY --from=builder /app/build /app/frontend
RUN mkdir /tmp_sounds

EXPOSE 5000

ENV FFMPEG_LOCATION="/usr/bin/ffmpeg"
ENV BUILD_DIR="/app/frontend"
ENV GENERATED_PATH="/tmp_sounds"
ENV BASE_PATH="/data"
ENV AUDIO_GEN_PATH="/app/audio_gen/startup.py"
ENV BITRATE="320k"
ENV RUST_BACKTRACE=1
ENV ROCKET_address=0.0.0.0

WORKDIR /app

CMD ["/app/target/release/backend"]