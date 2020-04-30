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
FROM ubuntu:18.04
RUN apt-get update && apt-get install -y python3-pip python3-dev ffmpeg

COPY ./backend /app

COPY --from=builder /app/build /frontend
RUN mkdir /tmp_sounds

WORKDIR /app
RUN pip3 install -r requirements.txt
RUN mkdir sounds/

EXPOSE 5000

ENV FFMPEG_LOCATION="/usr/bin/ffmpeg"
ENV SERVE="true"
ENV STATIC_FOLDER="/frontend"
ENV TMP_PATH="/tmp_sounds"

ENTRYPOINT [ "python3" ]
CMD [ "startup.py" ]