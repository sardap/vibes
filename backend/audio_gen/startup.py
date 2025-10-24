import itertools
from pydub import AudioSegment
from threading import Lock, Thread

import ntpath
import os
import json
import enum

SAMPLE_LENGTH = 600 * 1000


COMPLETE_SET = []

SOUND_DIR_PATH = os.environ["SOUND_PATH"]
FFMEPG_LOCATION = os.environ["FFMPEG_LOCATION"]
GENERATE_PATH = os.environ["GENERATED_PATH"]


BITRATE = os.environ.get("BITRATE", "48k")


LOW_WETHER_DB = os.environ.get("LOW_WETHER_DB", -85)
MED_WEATHER_DB = os.environ.get("MED_WEATHER_DB", -45)
HIGH_WEATHER_DB = os.environ.get("HIGH_WEATHER_DB", -25)

_weather_effects = {}

_cache = {}


class CloudState(enum.Enum):
    Nothing = 0
    Sunny = 1
    Overcast = 2


class WeatherAmount(enum.Enum):
    Nothing = 0
    Low = 1
    Med = 2
    High = 3


class Weather:
    cloud_state = CloudState.Sunny
    raining = WeatherAmount.Nothing
    snowing = WeatherAmount.Nothing

    def __init__(self, api_output=None, from_dict=None):
        if from_dict != None:
            self.cloud_state = CloudState(from_dict["weather"]["cloud"])
            self.raining = WeatherAmount(from_dict["weather"]["raining"])
            self.snowing = WeatherAmount(from_dict["weather"]["snowing"])

        if api_output == None:
            return

        weather_body = api_output["weather"][0]

        # Raining
        if weather_body["id"] in [500, 511, 300, 301, 302, 310, 311, 313, 200, 230]:
            self.raining = WeatherAmount.Low

        if weather_body["id"] in [501, 520, 531, 521, 201, 231, 232, 313, 314, 321]:
            self.raining = WeatherAmount.Med

        if weather_body["id"] in [502, 503, 522, 202]:
            self.raining = WeatherAmount.High

        # Snowing
        if weather_body["id"] in [601, 612, 615, 616]:
            self.snowing = WeatherAmount.Low

        if weather_body["id"] in [621, 601]:
            self.snowing = WeatherAmount.Med

        if weather_body["id"] in [602, 622]:
            self.snowing = WeatherAmount.High

        if (
            weather_body["id"] in [804, 500, 511, 520, 521, 522, 531]
            or round(weather_body["id"] / 100) == 6
            or round(weather_body["id"] / 100) == 3
            or round(weather_body["id"] / 100) == 2
        ):
            self.cloud_state = CloudState.Overcast

    def __dict__(self):
        return {
            "cloud": self.cloud_state.value,
            "raining": self.raining.value,
            "snowing": self.snowing.value,
        }

    def __str__(self):
        return "{},{},{}".format(self.cloud_state, self.raining, self.snowing)

    def export(self):
        return self.__dict__()

    def music_type(self):
        if self.raining != WeatherAmount.Nothing:
            return "Rain"

        if self.snowing != WeatherAmount.Nothing:
            return "Snow"

        return "none"


def load_config():
    global COMPLETE_SET

    with open(os.environ["CONFIG_PATH"]) as f:
        result = json.loads(f.read())

    for key, value in result["weather_effects"].items():
        filename, file_extension = os.path.splitext(value)
        _weather_effects[key] = AudioSegment.from_file(
            os.path.join(SOUND_DIR_PATH, value), format=file_extension[1:]
        )

    COMPLETE_SET = result["sets"]

    return result


_config = load_config()


def pad_sample(sample: AudioSegment, target_length_ms=10000) -> AudioSegment:
    base_len = len(sample)

    while len(sample) < target_length_ms:
        sample = sample.append(sample, crossfade=base_len * 0.05)

    if len(sample) > target_length_ms:
        sample = sample[:target_length_ms]
        sample.fade_out(5000)

    return sample


def set_level(sample: AudioSegment, target=-25) -> AudioSegment:
    return sample.apply_gain(target - sample.dBFS)


def gen_sample(input_file: str, export_path):
    _, file_extension = os.path.splitext(input_file)

    input_file_splits = input_file.split("/")
    input_file = (os.path.sep + "").join(
        input_file_splits,
    )
    path = os.path.join(SOUND_DIR_PATH, input_file)

    sample: AudioSegment = AudioSegment.from_file(path, format=file_extension[1:])

    sample = set_level(sample)
    sample = pad_sample(sample, SAMPLE_LENGTH)

    # Fade out
    sample = sample.append(AudioSegment.silent(duration=5000), 5000)
    sample = AudioSegment.silent(duration=5000).append(sample, 5000)

    sample.export(export_path, format="ogg", bitrate=BITRATE)


gen_lock = Lock()
generating = {}


def get_time_music(hour, set, weather_state):
    print(f"Getting time music {hour}, {set}, {weather_state}")
    set_music = _config["music"][set]

    hourStr = str(hour)

    if hasattr(set_music[hourStr], "get"):
        next_file = set_music[hourStr].get(
            weather_state.music_type(), set_music[hourStr]["none"]
        )
    else:
        next_file = set_music[hourStr]

    head, tail = ntpath.split(next_file)
    tail = tail[:-3]
    tail = tail + "ogg"
    file_path = os.path.join(GENERATE_PATH, tail or ntpath.basename(head))

    should_gen = False

    gen_lock.acquire()
    try:
        if not os.path.exists(file_path) and file_path not in generating:
            generating[file_path] = True
            should_gen = True
    finally:
        gen_lock.release()

    if should_gen:
        gen_sample(next_file, file_path)

    return file_path


def change_effect_level(effect, amount):
    return set_level(
        effect,
        target={
            WeatherAmount.Low: LOW_WETHER_DB,
            WeatherAmount.Med: MED_WEATHER_DB,
            WeatherAmount.High: HIGH_WEATHER_DB,
        }[amount],
    )


def get_effects_for_weather(weather_state=Weather()):
    effects = []

    if weather_state.raining != WeatherAmount.Nothing:
        effects.append(
            change_effect_level(_weather_effects["Rain"], weather_state.raining)
        )

    if weather_state.snowing != WeatherAmount.Nothing:
        effects.append(
            change_effect_level(_weather_effects["Snow"], weather_state.snowing)
        )

    return effects


def get_weather_effects_file(weather_state=Weather(), duration=0):
    def is_expired(key):
        return False

    if str(weather_state) not in _cache:
        sample = AudioSegment.silent(duration=duration)

        effects = get_effects_for_weather(weather_state)

        for effect in effects:
            sample = sample.overlay(effect, loop=True)

        file_name = os.path.join(GENERATE_PATH, "%s.mp3" % (str(weather_state)))
        sample.export(out_f=file_name, format="mp3", bitrate=BITRATE)

        _cache[str(weather_state)] = {
            "file_location": file_name,
            "is_expired": is_expired,
        }

    return _cache[str(weather_state)]["file_location"]


def main():
    print("Running audio_gen")
    AudioSegment.converter = FFMEPG_LOCATION

    weather_set = [Weather()]

    rain_and_snow_set = list(
        itertools.permutations(
            [
                WeatherAmount.Nothing,
                WeatherAmount.Low,
                WeatherAmount.Med,
                WeatherAmount.High,
            ],
            2,
        )
    )

    for cloud_state in [CloudState.Nothing, CloudState.Overcast, CloudState.Sunny]:
        for rain_and_snow in rain_and_snow_set:
            weather_set.append(
                Weather(
                    from_dict={
                        "weather": {
                            "cloud": cloud_state.value,
                            "raining": rain_and_snow[0].value,
                            "snowing": rain_and_snow[1].value,
                        }
                    }
                ),
            )

    print("Generating audio")
    threads = []
    for i in range(0, 24):
        for music_set in COMPLETE_SET:
            for weather in weather_set:
                t = Thread(target=get_time_music, args=(i, music_set, weather))
                t.start()
                threads.append(t)
    for t in threads:
        t.join()


if __name__ == "__main__":
    main()
