from threading import Thread, Semaphore
from pydub import AudioSegment
from pydub.playback import play
from os import walk
from datetime import datetime
from datetime import timedelta
from time import sleep
from flask import Flask, send_file, jsonify, make_response, request, send_from_directory, redirect
from logging.handlers import RotatingFileHandler
from waitress import serve

import ntpath
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
import base64

from forms import CreateSampleForm, LoginForm, GetSampleForm, GetSetForm

SAMPLE_LENGTH = 600 * 1000

app = Flask(__name__, static_folder=os.environ.get("STATIC_FOLDER", ""))

COMPLETE_SET = []
WEATHER_API_KEY = os.environ["WEATHER_API_KEY"]
WEATHER_API_ENDPOINT = os.environ["WEATHER_API_ENDPOINT"]
SOUND_DIR_PATH = os.environ["SOUND_PATH"]
FFMEPG_LOCATION = os.environ["FFMPEG_LOCATION"]
BITRATE = os.environ.get("BITRATE", "48k")
PORT = os.environ.get("PORT", 5000)

TMP_PATH = os.environ["TMP_PATH"]

LOW_WETHER_DB = os.environ.get("LOW_WETHER_DB", -85)
MED_WEATHER_DB = os.environ.get("MED_WEATHER_DB", -45)
HIGH_WEATHER_DB = os.environ.get("HIGH_WEATHER_DB", -25)

_weather_effects = {}
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

	def __init__(self, api_output=None, from_dict=None):
		if(from_dict != None):
			self.cloud_state = CloudState(from_dict["weather"]["cloud"])
			self.raining = WeatherAmount(from_dict["weather"]["raining"])
			self.snowing = WeatherAmount(from_dict["weather"]["snowing"])

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
			weather_body["id"] in [804, 500, 511, 520, 521, 522, 531] or
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
def def_path(path):
	if((path == "/" or path == "" ) and "access_key" not in request.args):
		return redirect("/login")

	if path != "" and os.path.exists(os.path.join(app.static_folder, path).strip()):
		return send_from_directory(app.static_folder, path)
	else:
		return send_from_directory(app.static_folder, 'index.html')

@app.route("/api/get_sample_length")
def get_sample_length_endpoint():
	return make_response(
		jsonify({
			"length_ms" : SAMPLE_LENGTH,
		}),
		400
	)


@app.route("/api/get_bell")
def get_bell_endpoint():
	return send_file(
		os.path.join(SOUND_DIR_PATH, _config["bell_sound"]),
		attachment_filename="bell_sound.mp3",
		mimetype="audio/mp3"
	)

@app.route("/api/get_weather_effect/<workaround>")
def get_weather_effect_endpoint(workaround):
	form = LoginForm(request.args)

	if(not form.validate() or form.access_key.data not in _access_keys):
		return make_response(
			jsonify({
				"error" : form.errors
			}),
			400
		)

	weather = get_weather_for_city(city_name=form.city_name.data, country_code=form.country_code.data)

	return send_file(
		get_weather_effects_file(weather_state=weather, duration=10 * 60 * 1000),
		attachment_filename="mp3",
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

@app.route("/api/get_sample/<name>/<hour>", methods=["GET"])
def get_sample_endpoint(name, hour):
	form = GetSampleForm(request.args)
	if(
		not form.validate() or 
		name not in COMPLETE_SET or 
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
	music_path = get_time_music(hour=hour, set=name, weather_state=weather)
	
	response = make_response(send_file(
		music_path,
		mimetype="audio/ogg",
		conditional=True,
	))

	return response

@app.route("/api/get_set", methods=["GET"])
def get_set_endpoint():
	form = GetSetForm(request.args)
	if(
		not form.validate() or 
		form.access_key.data not in _access_keys
	):
		return make_response(
			jsonify({
				"error" : form.errors
			}),
			400
		)

	return make_response(
		jsonify(
			COMPLETE_SET
		), 
		200
	)

def pad_sample(sample, target_length_ms=10000):
	base_len = len(sample)

	while(len(sample) < target_length_ms):
		sample = sample.append(sample, crossfade=base_len * 0.05)

	if(len(sample) > target_length_ms):
		sample = sample[:target_length_ms]
		sample.fade_out(5000)

	return sample

def get_weather_for_city(city_name, country_code):
	url = "%s/data/2.5/weather?q=%s,%s&appid=%s" % (WEATHER_API_ENDPOINT, city_name, country_code, WEATHER_API_KEY)

	response = requests.get(url, headers={"easy_cache_expire_second" : str(10 * 60)})
	body = response.json()

	return Weather(api_output=body)

def load_config():
	global COMPLETE_SET

	with open(os.environ["CONFIG_PATH"]) as f:
		result = json.loads(f.read())

	for key, value in result["weather_effects"].items():
		filename, file_extension = os.path.splitext(value)
		_weather_effects[key] = AudioSegment.from_file(os.path.join(SOUND_DIR_PATH, value), format=file_extension[1:])

	for key in result["access_keys"]:
		_access_keys.append(key)
	
	COMPLETE_SET = result["sets"]

	return result

_config = load_config()

def set_level(sample, target=-25):
	return sample.apply_gain(target - sample.dBFS)

def gen_sample(input_file, export_path):
	app.logger.info("Loading %s from file" % (input_file))
	file_name, file_extension = os.path.splitext(input_file)
	sample = AudioSegment.from_file(os.path.join(SOUND_DIR_PATH, input_file), format=file_extension[1:])

	sample = set_level(sample)
	sample = pad_sample(sample, SAMPLE_LENGTH)

	#Fade out
	sample = sample.append(AudioSegment.silent(duration=5000), 5000)
	sample = AudioSegment.silent(duration=5000).append(sample, 5000)

	sample.export(export_path, format="ogg", bitrate=BITRATE)

def get_time_music(hour, set, weather_state):
	set_music = _config["music"][set]

	hourStr = str(hour)

	if(hasattr(set_music[hourStr], "get")):
		next_file = set_music[hourStr].get(weather_state.music_type(), set_music[hourStr]["none"])
	else:
		next_file = set_music[hourStr]
	
	head, tail = ntpath.split(next_file)
	tail = tail[:-3]
	tail = tail + "ogg"
	file_path = os.path.join(TMP_PATH, tail or ntpath.basename(head))
	
	if(not os.path.exists(file_path)):
		gen_sample(next_file, file_path)

	return file_path

def change_effect_level(effect, amount):
	return set_level(
		effect, 
		target={
			WeatherAmount.Low : LOW_WETHER_DB,
			WeatherAmount.Med : MED_WEATHER_DB,
			WeatherAmount.High : HIGH_WEATHER_DB
		}[amount]
	)

def get_effects_for_weather(weather_state=Weather()):
	effects = []

	if(weather_state.raining != WeatherAmount.Nothing):
		effects.append(change_effect_level(_weather_effects["Rain"], weather_state.raining))

	if(weather_state.snowing != WeatherAmount.Nothing):
		effects.append(change_effect_level(_weather_effects["Snow"], weather_state.snowing))

	return effects

def get_weather_effects_file(weather_state=Weather(), duration=0):
	def is_expired(key):
		return False

	if(str(weather_state) not in _cache):
		sample = AudioSegment.silent(duration=duration)

		effects = get_effects_for_weather(weather_state)

		for effect in effects:
			sample = sample.overlay(effect, loop=True)

		file_name = os.path.join(TMP_PATH, "%s.mp3" % (str(weather_state)))
		sample.export(out_f=file_name, format="mp3", bitrate=BITRATE)

		_cache[str(weather_state)] = {
			"file_location" : file_name,
			"is_expired" : is_expired,
		}
		
	return _cache[str(weather_state)]["file_location"]

def backgroundgen():
	for i in range(0, 24):
		for music_set in COMPLETE_SET:
			# Thread(target=get_time_music, args=(i, music_set, Weather())).start()
			get_time_music(hour=i, set=music_set, weather_state=Weather())

def main():
	AudioSegment.converter = FFMEPG_LOCATION

	Thread(target=backgroundgen).start()

	logging.basicConfig(level=logging.DEBUG)

	if os.environ.get("SERVE", False):
		serve(app, host="0.0.0.0", port=PORT)

if __name__ == "__main__":
	main()