import React from 'react';
import Button from 'react-bootstrap/Button';
import Spinner from 'react-bootstrap/Spinner'
import Slider from 'react-rangeslider'
import { useAsync } from "react-async"
import CheckIcon from '@material-ui/icons/Check';
import ToggleButton from '@material-ui/lab/ToggleButton';
import invert from 'invert-color';

import './App.css'
import 'react-rangeslider/lib/index.css'
import 'bootstrap/dist/css/bootstrap.min.css';

var SunCalc = require('suncalc');

class MusicPlayer extends React.Component {
	constructor(props) {
		super(props);

		this.enabled_games = ["wild_world", "city_folk", "new_leaf", "gamecube", "new_horizons"];

		const urlParams = new URLSearchParams(window.location.search);
		this.country_code = urlParams.get('country_code');
		this.city = urlParams.get('city');
		this.access_key = urlParams.get('access_key');

		this.state = {
			loading: false,
			current_time: this.get_current_time_string(),
			volume: 100,
			timer_text_color: "#000000",
			playing: false,
			wacky: false
		};

		navigator.geolocation.getCurrentPosition(position => {
			this.setState({
				lat: position.coords.latitude,
				lng: position.coords.longitude
			});
		});

		this.bell_playing = false
		this.bell_played = false
		this.vol_mod = 1

		this.clock_update_interval_id = setInterval(this.clock_update.bind(this), 1000);
		this.weather_update_interval_id = setInterval(this.weather_update.bind(this), 1000 * 60 * 10 + 1000);
	}

	componentDidMount() {
		this.weather_audio = new Audio();
		window.audio = new Audio();
		this.weather_update();
	 }  

	pad(num) {
		var result = num + "";
		while(result.length < 2) {
			result = "0" + result;
		}
		return result;
	}

	makeid(length) {
		var result           = '';
		var characters       = 'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789';
		var charactersLength = characters.length;
		for ( var i = 0; i < length; i++ ) {
		   result += characters.charAt(Math.floor(Math.random() * charactersLength));
		}
		return result;
	}
	

	play_weather_effect() {
		const weather_audio_src = window.location.origin + "/api/get_weather_effect/" + this.makeid(20) + "?" + 
			"access_key=" + this.access_key + "&" +
			"city_name=" +  this.city + "&" +
			"country_code=" + this.country_code;
		
		this.weather_audio.pause();
		this.weather_audio.src = weather_audio_src;
		this.weather_audio.play();
	}

	weather_update() {
		if(this.state.playing) {
			this.play_weather_effect();
		}

		const url = window.location.origin + "/api/get_weather" + "?" + 
			"access_key=" + this.access_key + "&" +
			"city_name=" +  this.city + "&" +
			"country_code=" + this.country_code;

		fetch(url)
			.then(function(data){
				return data.json();
			})
			.then((json) =>{
				this.setState({
					weather_state: json
				});
			});
	}

	get_current_time_string() {
		const now = new Date();
		return this.pad(now.getHours()) + ":" + this.pad(now.getMinutes()) + ":" + this.pad(now.getSeconds());
	}

	clock_update() {
		const background = this.get_background();
		document.body.style = 'background: ' + background + ';';
		
		if(background) {
			this.setState({
				current_time: this.get_current_time_string(),
				timer_text_color: this.getContrastYIQ(background)
			});
		}
	}

	rand(seed) {
		var lfsr = seed;
		var bit = 0;
		bit  = ((lfsr >> 0) ^ (lfsr >> 2) ^ (lfsr >> 3) ^ (lfsr >> 5) ) & 1;
		return lfsr =  (lfsr >> 1) | (bit << 15);
	}

	get_random_game() {
		const now = new Date();
		var x = 0;
		if(now.getMinutes() % 10 > 5){
			x = 1
		} else {
			x = 2
		}
		var seed = parseInt((now.getMinutes() + "")[0] + x + now.getDay() + now.getMonth() + now.getFullYear());
		var max = this.enabled_games.length;
		var min = 0;
		var idx = this.rand(seed) % (max - min + 1) + min;
		
		return this.enabled_games[idx];
	}

	start_vibing = () => {
		const now = new Date();

		this.play_weather_effect();

		window.audio.volume	= this.state.volume / 100;

		var hour = now.getHours()
		
		if(this.state.wacky) {
			if(hour >= 12) {
				hour = hour - 12
			} else {
				hour = hour + 12
			}
		}

		var game = this.get_random_game()
		console.log(game)

		const next_src = window.location.origin + "/api/get_sample/" + game + "/" + hour + "?" + 
			"access_key=" + this.access_key + "&" +
			"city_name=" +  this.city + "&" +
			"country_code=" + this.country_code;

		window.audio.pause();
		window.audio.src = next_src;
		window.audio.play();

		console.log("Playing next song! hour:" + hour + " wacky:" + this.state.wacky);

		this.setState({
			hour: now.getHours()
		});
	}

	music_update = () => {
	
		const now = new Date();

		if(this.bell_playing){
			if(window.audio.paused) {
				this.bell_playing = false
				this.vol_mod = 0
				this.updateVol()	
			}
			return
		}

		if(now.getMinutes() == 59) {
			this.vol_mod = 1 - now.getSeconds() / 60
			this.updateVol()
		}

		if(now.getMinutes() == 0) {
			this.vol_mod = now.getSeconds() / 60
			this.updateVol()
		}

		if(now.getHours() != this.state.hour && !this.bell_played) {
			this.bell_playing = true
			console.log("Playing Bell Sound!");
			this.vol_mod = 1
			this.updateVol()
			window.audio.pause();
			window.audio.src = window.location.origin + "/api/get_bell";
			window.audio.play();
			this.bell_played = true
			return
		}
		
		if(this.bell_played && now.getHours() != this.state.hour || window.audio.currentTime === window.audio.duration) {
			this.start_vibing();
			this.bell_played = false
		}
	}

	init_audio = () => {
		this.play_sound_interval_id = setInterval(this.music_update.bind(this), 1000);

		this.setState({
			playing: true
		});

		this.start_vibing();
	}

	componentToHex(c) {
		var hex = c.toString(16);
		return hex.length == 1 ? "0" + hex : hex;
	}
	  
	rgbToHex(r, g, b) {
		return "#" + this.componentToHex(r) + this.componentToHex(g) + this.componentToHex(b);
	}

	hexToRgb(hex) {
		var result = /^#?([a-f\d]{2})([a-f\d]{2})([a-f\d]{2})$/i.exec(hex);
		return result ? {
			r: parseInt(result[1], 16),
			g: parseInt(result[2], 16),
			b: parseInt(result[3], 16)
		} : null;
	}
	  
	get_color(now, start_time, end_time, start_color, middle_color, end_color) {
		const middle = new Date((start_time.getTime() + end_time.getTime()) / 2);
		
		if(now < middle) {
			start_color = this.hexToRgb(start_color);
			end_color = this.hexToRgb(middle_color);
			start_time = start_time;
			end_time = middle;
		} else {
			start_color = this.hexToRgb(middle_color);
			end_color = this.hexToRgb(end_color);
			start_time = middle;
			end_time = end_time;
		}

		const percent = Math.abs(start_time - now) / (end_time - start_time);

		var r = Math.round(start_color.r + percent * (end_color.r - start_color.r));
		var g = Math.round(start_color.g + percent * (end_color.g - start_color.g));
		var b = Math.round(start_color.b + percent * (end_color.b - start_color.b));

		return this.rgbToHex(r, g, b);
	}

	getContrastYIQ(hexcolor){
		hexcolor = hexcolor.replace("#", "");
		var r = parseInt(hexcolor.substr(0,2),16);
		var g = parseInt(hexcolor.substr(2,2),16);
		var b = parseInt(hexcolor.substr(4,2),16);
		var yiq = ((r*299)+(g*587)+(b*114))/1000;
		return (yiq >= 128) ? 'black' : 'white';
	}

	get_background() {
		if(!this.state.lat || !this.state.lng){
			return;
		}

		const now = new Date();
		const times = SunCalc.getTimes(now, this.state.lat, this.state.lng, now);

		let sky_day = "#87CEEB";
		if(this.state.weather_state) {
			const cloud_state = this.state.weather_state.weather.cloud;

			if(cloud_state == 2) {
				sky_day = "#D2D4D8";
			}
		}

		const sky_dusk = "#26556B";
		const sky_night = "#252526";
		const sky_duskset = "#2a424d"
		const sky_sunrise = "#FBAB17";
		const sky_sunset = "#FB8A14";
		const sky_dawn = "#0e3e53"
		const sky_dawnset = "#1e3641"
		var result;

		if(now > times.nightEnd && now < times.dawn) {
			// dawn
			result = this.get_color(now, times.nightEnd, times.dawn, sky_night, sky_dawnset, sky_dawn);
		} else if(now > times.dawn && now < times.sunriseEnd) {
			// Sunrise 
			result = this.get_color(now, times.dawn, times.sunriseEnd, sky_dawn, sky_sunrise, sky_day);
		} else if(now > times.sunriseEnd && now < times.sunsetStart) {
			// Daytime
			result = sky_day;
		} else if(now > times.sunsetStart && now < times.dusk) {
			// Sunset
			result = this.get_color(now, times.sunsetStart, times.dusk, sky_day, sky_sunset, sky_dusk);
		} else if(now > times.dusk && now < times.night) {
			// Dusk
			result = this.get_color(now, times.dusk, times.night, sky_dusk, sky_duskset, sky_night);
		} else {
			// Nighttime
			result = sky_night;
		}

		if(this.state.wacky){
			result = invert(result);
		}

		return result;
	}

	show_loading() {
		return (
			<div className="text-center">
				<Spinner animation="border" role="status">
					<span className="sr-only">Loading...</span>
				</Spinner>	
			</div>
		);
	}

	handleOnChange = (value) => {
		this.setState({
			volume: value
		})
		this.weather_audio.volume = (value / 100);
		this.updateVol()
	}
	
	updateVol = () => {
		window.audio.volume	= (this.state.volume / 100) * this.vol_mod;
	}

	get_house = () => {
		if(this.state.wacky){
			return"house_invert.png"
		}

		return "house.png"
	}

	render_toggle_button = () => {
		return <div className="text-center">
			<ToggleButton
				value="check"
				selected={this.state.wacky}
				onChange={() => {
					this.setState({
						wacky: !this.state.wacky
					}, () => {
						console.log("wacky: " + this.state.wacky)
						this.start_vibing()
					});
				}}
				>
				<CheckIcon />
			</ToggleButton>
		</div>

	}

	show_content() {
		let { volume } = this.state;

		return (
			<>
				<Button variant="outline-danger" onClick={this.state.playing ? this.pause_song : this.init_audio} disabled={this.state.playing}>
					<img src={this.get_house()} alt="Italian Trulli" />
					{this.state.playing ? <div>PLAYING</div> : <div>Not playing</div>}
				</Button>
				<Slider
					value={volume}
					orientation="horizontal"
					onChange={this.handleOnChange}
				/>
				{ this.state.playing ? this.render_toggle_button() : <></> }
			</>
		);
	}

	show_error() {
		return (
			<div>{this.state.error}</div>
		)
	}

	render() {
		return (
			<div>
				<div className="centered">
					<div className="text-center display-4" style={{color : this.state.timer_text_color}}>
						{new Date().toLocaleString('en-us', {  weekday: 'long' })}
					</div>
					<div className="text-center display-4" style={{color : this.state.timer_text_color}}>{this.state.current_time}</div>
					{this.state.loading ? this.show_loading() : (this.state.error ? this.show_error() : this.show_content()) }
				</div>
			</div>
		)
	}
}
  
export default MusicPlayer;