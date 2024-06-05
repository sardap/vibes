use once_cell::sync::Lazy;
use rocket::fs::NamedFile;
use serde::de::{self, Visitor};
use serde::{Deserialize, Deserializer, Serialize};
use std::path::PathBuf;
use std::process::Command;
use std::{collections::HashMap, fs, path::Path};
use std::{env, fmt};

#[macro_use]
extern crate rocket;

struct Arguments {
    generated_path: String,
    build_dir: String,
    audio_gen_path: String,
    config_path: String,
    weather_api_endpoint: String,
    weather_api_key: String,
    sounds_path: String,
}

fn load_collection() -> Collection {
    let data: String =
        fs::read_to_string(Path::new(&ARGUMENTS.config_path)).expect("Unable to read file");

    let collection: Collection = serde_json::from_str(&data).expect("JSON was not well-formatted");

    return collection;
}

fn load_arguments() -> Arguments {
    Arguments {
        generated_path: env::var("GENERATED_PATH").unwrap().to_string(),
        build_dir: env::var("BUILD_DIR").unwrap().to_string(),
        config_path: env::var("CONFIG_PATH").unwrap().to_string(),
        audio_gen_path: env::var("AUDIO_GEN_PATH").unwrap().to_string(),
        weather_api_endpoint: env::var("WEATHER_API_ENDPOINT").unwrap().to_string(),
        weather_api_key: env::var("WEATHER_API_KEY").unwrap().to_string(),
        sounds_path: env::var("SOUND_PATH").unwrap().to_string(),
    }
}

static ARGUMENTS: Lazy<Arguments> = Lazy::new(|| load_arguments());
static COLLECTION: Lazy<Collection> = Lazy::new(|| load_collection());

#[derive(Debug, Deserialize)]
struct WeatherEffects {
    rain: String,
    drizzle: String,
    thunderstorm: String,
}

#[derive(Debug, Deserialize, Serialize, Default)]
struct Weather {
    cloud: i32,
    raining: i32,
    snowing: i32,
}

#[derive(Debug, Deserialize, Serialize, Default)]
struct WeatherResponse {
    weather: Weather,
}

#[derive(Debug, Deserialize, Serialize)]
struct WeatherMapWeather {
    id: i32,
}

#[derive(Debug, Deserialize, Serialize)]
struct WeatherMapResponse {
    weather: Vec<WeatherMapWeather>,
}

impl Weather {
    async fn get_weather_for_country(country_code: String, city_name: String) -> Self {
        Weather::default()
        /*
        let url = format!(
            "{}/data/2.5/weather?q={},{}&appid={}",
            ARGUMENTS.weather_api_endpoint, city_name, country_code, ARGUMENTS.weather_api_key,
        );

        let resp = reqwest::get(url).await.expect("Fuck");
        let text = resp.text().await.unwrap();

        let weather: WeatherMapResponse =
            serde_json::from_str(&text).expect("JSON was not well-formatted");

        let mut result = Weather::default();

        let weather_id = weather.weather[0].id;

        result.raining = match weather_id {
            500 | 511 | 300 | 301 | 302 | 310 | 311 | 313 | 200 | 230 => 1,
            501 | 520 | 531 | 521 | 201 | 231 | 232 | 314 | 321 => 2,
            502 | 503 | 522 | 202 => 3,
            _ => 0,
        };

        result.snowing = match weather_id {
            612 | 615 | 616 => 1,
            621 | 601 => 2,
            602 | 622 => 3,
            _ => 0,
        };

        result.cloud = match weather_id {
            804 | 500 | 511 | 520 | 521 | 522 | 531 => 2,
            _ => 1,
        };

        result.cloud = match weather_id / 100 {
            6 | 3 | 2 => 2,
            _ => result.cloud,
        };

        result
        */
    }
}

#[derive(Debug, Default)]
struct HourSet {
    none: String,
    rain: String,
    drizzle: String,
    thunderstorm: String,
    snow: String,
}

impl HourSet {
    fn get_sample(&self, weather: &Weather) -> &str {
        if weather.raining > 0 {
            return &self.rain;
        }

        if weather.snowing > 0 {
            return &self.snow;
        }

        return &self.none;
    }
}

struct ArrayKeyedMapDeserializer;

impl<'de> Visitor<'de> for ArrayKeyedMapDeserializer {
    type Value = HourSet;

    fn expecting(&self, formatter: &mut fmt::Formatter) -> fmt::Result {
        formatter.write_str("ArrayKeyedMap key value sequence.")
    }

    fn visit_str<E>(self, s: &str) -> Result<Self::Value, E>
    where
        E: de::Error,
    {
        Ok(HourSet {
            rain: s.to_string(),
            none: s.to_string(),
            thunderstorm: s.to_string(),
            snow: s.to_string(),
            drizzle: s.to_string(),
        })
    }

    fn visit_map<A>(self, mut map: A) -> Result<Self::Value, A::Error>
    where
        A: de::MapAccess<'de>,
    {
        let mut result = HourSet::default();
        let mut complete = false;
        while !complete {
            match map.next_entry::<String, String>() {
                Ok(entry) => match entry {
                    Some(pair) => match pair.0.as_str() {
                        "rain" => result.rain = pair.1,
                        "none" => result.none = pair.1,
                        "thunderstorm" => result.thunderstorm = pair.1,
                        "drizzle" => result.drizzle = pair.1,
                        "snow" => result.snow = pair.1,
                        _ => {}
                    },
                    None => complete = true,
                },
                Err(_) => todo!(),
            }
        }

        Ok(result)
    }
}

impl<'de> Deserialize<'de> for HourSet {
    fn deserialize<D>(deserializer: D) -> Result<Self, D::Error>
    where
        D: Deserializer<'de>,
    {
        deserializer.deserialize_any(ArrayKeyedMapDeserializer)
    }
}

#[derive(Debug, Deserialize)]
struct Collection {
    bell_sound: String,
    weather_effects: WeatherEffects,
    sets: Vec<String>,
    music: HashMap<String, HashMap<String, HourSet>>,
}

#[get("/api/get_set")]
fn endpoint_get_set() -> String {
    let result = serde_json::to_string(&COLLECTION.sets).unwrap();
    result
}

#[get("/api/get_weather/<country_code>/<city_name>")]
async fn endpoint_get_weather(country_code: String, city_name: String) -> String {
    let weather = Weather::get_weather_for_country(country_code, city_name).await;
    let weather_response = WeatherResponse { weather };
    let result = serde_json::to_string(&weather_response).unwrap();
    result
}

#[get("/api/get_weather_effect/<_workaround>/<country_code>/<city_name>")]
async fn endpoint_get_weather_effect(
    _workaround: String,
    country_code: String,
    city_name: String,
) -> NamedFile {
    let weather = Weather::get_weather_for_country(country_code, city_name).await;

    let mut weather_event_file = "".to_string();

    if weather.snowing > 0 {
        weather_event_file = COLLECTION.weather_effects.thunderstorm.clone();
    } else if weather.raining > 1 {
        weather_event_file = COLLECTION.weather_effects.rain.clone();
    } else if weather.raining > 0 {
        weather_event_file = COLLECTION.weather_effects.drizzle.clone();
    }

    let result = NamedFile::open(Path::new(&ARGUMENTS.sounds_path).join(&weather_event_file))
        .await
        .ok()
        .unwrap();
    result
}

#[get("/api/get_sample/<country_code>/<city_name>/<name>/<hour>")]
async fn endpoint_get_sample(
    country_code: String,
    city_name: String,
    name: String,
    hour: String,
) -> NamedFile {
    let weather = Weather::get_weather_for_country(country_code, city_name).await;

    let file_name = COLLECTION
        .music
        .get(&name)
        .unwrap()
        .get(&hour)
        .unwrap()
        .get_sample(&weather);

    let sound_file_name = file_name.split("/").last().unwrap();
    let mut path = Path::new(&ARGUMENTS.generated_path).join(sound_file_name);
    path.set_extension("ogg");

    let result = NamedFile::open(path).await.ok().unwrap();
    result
}

#[get("/api/get_bell")]
async fn endpoint_get_bell() -> NamedFile {
    let result = NamedFile::open(Path::new(&ARGUMENTS.sounds_path).join(&COLLECTION.bell_sound))
        .await
        .ok()
        .unwrap();
    result
}

#[get("/<file..>")]
async fn build_dir(file: PathBuf) -> Option<NamedFile> {
    NamedFile::open(Path::new(&ARGUMENTS.build_dir).join(file))
        .await
        .ok()
}

#[get("/")]
async fn index() -> NamedFile {
    NamedFile::open(Path::new(&ARGUMENTS.build_dir).join("index.html"))
        .await
        .ok()
        .unwrap()
}

#[launch]
fn rocket() -> _ {
    println!("Running {}", &ARGUMENTS.audio_gen_path);
    let mut gen_cmd = Command::new("python3")
        .arg(&ARGUMENTS.audio_gen_path)
        .spawn()
        .expect("Unable to generate audio");

    let _result = gen_cmd.wait().unwrap();

    println!("Sets {:?}", COLLECTION.sets);

    rocket::build().mount(
        "/",
        routes![
            index,
            build_dir,
            endpoint_get_set,
            endpoint_get_weather,
            endpoint_get_weather_effect,
            endpoint_get_bell,
            endpoint_get_sample
        ],
    )
}
