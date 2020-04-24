from threading import Thread, Semaphore
from pydub import AudioSegment
from pydub.playback import play
from os import walk
from datetime import datetime
from datetime import timedelta
from time import sleep
from flask import Flask, send_file, jsonify, make_response, request, send_from_directory, redirect
from logging.handlers import RotatingFileHandler

import logging
import requests
import random
import string
import math
import os
import json
import io
import base64
import pytz
import uuid 
import enum

from forms import CreateSampleForm, LoginForm, GetSampleForm

SAMPLE_LENGTH = 300 * 1000

app = Flask(__name__, static_folder=os.environ.get("STATIC_FOLDER", ""))

ENABLED_GAMES = ["new_leaf", "wild_world", "city_folk", "gamecube", "new_horizons"]
CACHE_REFRESH_TIME = 5 * 60
WEATHER_API_KEY = os.environ["WEATHER_API_KEY"]
SOUND_DIR_PATH = os.environ["SOUND_PATH"]
FFMEPG_LOCATION = os.environ["FFMPEG_LOCATION"]
BITRATE = os.environ.get("BITRATE", "48k")

LOW_WETHER_DB = os.environ.get("LOW_WETHER_DB", -45)
MED_WEATHER_DB = os.environ.get("MED_WEATHER_DB", -25)
HIGH_WEATHER_DB = os.environ.get("HIGH_WEATHER_DB", -10)

_next_sample = None
_weather_effects = {}
_sample_sem = Semaphore(1)
_file_lock = Semaphore(1)
_access_keys = []

_cache = {}
_user_samples = {}

class CloudState(enum.Enum):
	Nothing = 0
	Sunny = 1
	Overcast = 2

class WeatherAmount(enum.Enum):
	Nothing = 0
	Low = 1
	Med = 2
	High = 3

class Weather():
	cloud_state = CloudState.Sunny
	raining = WeatherAmount.Nothing
	snowing = WeatherAmount.Nothing

	def __init__(self, api_output=None):
		if(api_output == None):
			return

		weather_body = api_output["weather"][0]

		# Raining
		if(weather_body["id"] in [500, 511, 300, 301, 302, 310, 311, 313, 200, 230]):
			self.raining = WeatherAmount.Low

		if(weather_body["id"] in [501, 520, 531, 521, 201, 231, 232, 313, 314, 321]):
			self.raining = WeatherAmount.Med
		
		if(weather_body["id"] in [502, 503, 522, 202]):
			self.raining = WeatherAmount.High

		# Snowing
		if(weather_body["id"] in [601, 612, 615, 616]):
			self.snowing = WeatherAmount.Low

		if(weather_body["id"] in [621, 601]):
			self.snowing = WeatherAmount.Med

		if(weather_body["id"] in [602, 622]):
			self.snowing = WeatherAmount.High

		if(
			weather_body["id"] in [803, 804, 511, 520, 521, 522, 531] or
			round(weather_body["id"] / 100) == 6 or
			round(weather_body["id"] / 100) == 3 or 
			round(weather_body["id"] / 100) == 2
		):
			self.cloud_state = CloudState.Overcast

	def __dict__(self):
		return {
			"cloud" : self.cloud_state.value,
			"raining" : self.raining.value,
			"snowing" : self.snowing.value
		}

	def __str__(self):
		return "{},{},{}".format(self.cloud_state, self.raining, self.snowing)

	def export(self):
		return self.__dict__()

	def music_type(self):
		if(self.raining != WeatherAmount.Nothing):
			return "Rain"
		
		if(self.snowing != WeatherAmount.Nothing):
			return "Snow"

		return "none"

@app.route('/', defaults={'path': ''})
@app.route('/<path:path>')
def serve(path):
	if((path == "/" or path == "" ) and "access_key" not in request.args):
		return redirect("/login")

	if path != "" and os.path.exists(os.path.join(app.static_folder, path).strip()):
		return send_from_directory(app.static_folder, path)
	else:
		return send_from_directory(app.static_folder, 'index.html')

@app.route("/api/get_bell")
def get_bell_endpoint():
	return send_file(
		os.path.join(SOUND_DIR_PATH, _config["bell_sound"]),
		attachment_filename="bell_sound.mp3",
		mimetype="audio/mp3"
	)

@app.route("/api/get_weather")
def get_weather_endpoint():
	form = LoginForm(request.args)

	if(not form.validate() or form.access_key.data not in _access_keys):
		return make_response(
			jsonify({
				"error" : form.errors
			}),
			400
		)

	result = get_weather_for_city(city_name=form.city_name.data, country_code=form.country_code.data)

	return make_response(
		jsonify({
			"weather" : result.export()
		}),
		200
	)


@app.route("/api/login", methods=["POST"])
def login_endpoint():
	form = LoginForm(request.form)

	if(not form.validate()):
		app.logger.info("Failed login {}".format(form.errors))
		return redirect("/login")

	if(form.access_key.data not in _access_keys):
		return redirect("/login")

	target = "/?access_key={}&city={}&country_code={}".format(
		form.access_key.data,
		form.city_name.data,
		form.country_code.data
	)

	return redirect(
		target
	)

@app.route("/api/get_sample/<game_name>/<hour>", methods=["GET"])
def get_sample_endpoint(game_name, hour):
	form = GetSampleForm(request.args)

	if(
		not form.validate() or 
		game_name not in ENABLED_GAMES or 
		(not (int(hour) >= 0 and int(hour) <= 24)) or 
		form.access_key.data not in _access_keys
	):
		return make_response(
			jsonify({
				"error" : form.errors
			}),
			400
		)

	weather = get_weather_for_city(city_name=form.city_name.data, country_code=form.country_code.data)
	sample = get_time_music(hour=hour, game=game_name, weather_state=weather)
	
	return send_file(
		io.BytesIO(sample.export(format="mp3", bitrate=BITRATE).read()),
		attachment_filename="new_sound.mp3",
		mimetype="audio/mp3"
	)

def pad_sample(sample=None, target_length_ms=10000):
	base_len = len(sample)

	while(len(sample) < target_length_ms):
		sample = sample.append(sample, crossfade=base_len * 0.05)

	return sample

def get_sample(next_file, weather_state=None):
	def is_expired(key):
		return (datetime.utcnow() - _cache[key]["last_access_time"]).total_seconds() > 10 * 60

	file_key = next_file

	if(weather_state != None):
		file_key = "%s:%s" % (next_file, str(weather_state))

	_file_lock.acquire()
	try:
		if(file_key not in  _cache):
			app.logger.info("Loading %s from file" % (next_file))
			filename, file_extension = os.path.splitext(next_file)
			sample = AudioSegment.from_file(os.path.join(SOUND_DIR_PATH, next_file), format=file_extension[1:])

			sample = pad_sample(sample, SAMPLE_LENGTH)

			sample = set_level(sample)

			if(weather_state != None):
				sample = apply_weather_effects(sample=sample, weather_state=weather_state)

			_cache[file_key] = {
				"sample" : sample,
				"last_access_time" : datetime.utcnow(),
				"is_expired" : is_expired
			}

			app.logger.info("Added %s to cache" % (file_key))
	finally:
		_file_lock.release()

	return _cache[file_key]["sample"]

def get_weather_for_city(city_name=None, country_code=None):
	def is_expired(key):
		return datetime.utcnow() > _cache[key]["expire_time"]

	_file_lock.acquire()
	try:
		key = (city_name + country_code).lower()
		if(key not in _cache):
			app.logger.info("%s Not in weather cache adding" % key)
			url = "https://api.openweathermap.org/data/2.5/weather?q=%s,%s&appid=%s" % (city_name, country_code, WEATHER_API_KEY)

			response = requests.get(url)
			body = response.json()
			_cache[key] = {
				"data" : body["weather"][0],
				"weather_state" : Weather(api_output=body),
				"expire_time" : datetime.utcnow() + timedelta(minutes=10),
				"is_expired" : is_expired
			}

		result = _cache[key]["weather_state"]
	finally:
		_file_lock.release()

	return result
	

def load_config():
	with open(os.environ["CONFIG_PATH"]) as f:
		result = json.loads(f.read())

	for key, value in result["weather_effects"].items():
		filename, file_extension = os.path.splitext(value)
		_weather_effects[key] = AudioSegment.from_file(os.path.join(SOUND_DIR_PATH, value), format=file_extension[1:])

	for key in result["access_keys"]:
		_access_keys.append(key)

	return result

_config = load_config()

def set_level(sample, target=-25):
	return sample.apply_gain(target - sample.dBFS)

def get_time_music(hour=None, game=None, weather_state=None):
	game_music = _config["music"][game]

	if(hasattr(game_music[str(hour)], "get")):
		next_file = game_music[str(hour)].get(weather_state.music_type(), game_music[str(hour)]["none"])
	else:
		next_file = game_music[str(hour)]
	
	return get_sample(next_file, weather_state=weather_state)

def apply_weather_effects(weather_state=Weather(), sample=None):
	def change_level(effect, amount):
		return set_level(
			effect, 
			target={
				WeatherAmount.Low : LOW_WETHER_DB,
				WeatherAmount.Med : MED_WEATHER_DB,
				WeatherAmount.High : HIGH_WEATHER_DB
			}[amount]
		)

	effect = None

	if(weather_state.raining != WeatherAmount.Nothing):
		effect = change_level(_weather_effects["Rain"], weather_state.raining)
	elif(weather_state.snowing != WeatherAmount.Nothing):
		effect = change_level(_weather_effects["Snow"], weather_state.snowing)

	if(effect != None):
		return sample.overlay(effect, loop=True)
	
	return sample

def cache_clear():
	while True:
		app.logger.info("cache starting check")
		keys = list(_cache.keys())
		for key in keys:
			_file_lock.acquire()
			try:
				if(_cache[key]["is_expired"](key)):
					app.logger.info("cache removing {}".format(key))
					del _cache[key]
			finally:
				_file_lock.release()

		keys = list(_user_samples)
		for key in keys:
			if(datetime.utcnow() > _user_samples[key]["expire_time"]):
				del _user_samples[key]

		app.logger.info("Clearing unused samples")
		sleep(CACHE_REFRESH_TIME)

def main():
	AudioSegment.converte1r = FFMEPG_LOCATION

	if __name__ != "__main__":
		return

	Thread(target=cache_clear).start()

	logging.basicConfig(level=logging.DEBUG)

	if os.environ.get("SERVE", False):
		app.run(host="0.0.0.0", port=5000, debug=False)

main()