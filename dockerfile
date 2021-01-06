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
FROM python:latest

ENV DEBIAN_FRONTEND=noninteractive

RUN apt-get update && apt-get install -y ffmpeg

COPY --from=builder /app/build /app/frontend
RUN mkdir /tmp_sounds

WORKDIR /app
COPY ./backend/requirements.txt /app/requirements.txt
RUN pip3 install -r requirements.txt

COPY ./backend/ /app/

RUN mkdir sounds/

EXPOSE 5000

ENV FFMPEG_LOCATION="/usr/bin/ffmpeg"
ENV SERVE="true"
ENV STATIC_FOLDER="/app/frontend"
ENV TMP_PATH="/tmp_sounds"

CMD ["python3", "startup.py"]
