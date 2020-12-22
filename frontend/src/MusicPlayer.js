import React from 'react';
import Button from 'react-bootstrap/Button';
import Spinner from 'react-bootstrap/Spinner'
import Slider from 'react-rangeslider'
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

		this.enabled_sets = [];

		const urlParams = new URLSearchParams(window.location.search);
		this.country_code = urlParams.get('country_code');
		this.city = urlParams.get('city');
		this.access_key = urlParams.get('access_key');

		this.state = {
			loading: false,
			current_time: this.getCurrentTimeString(),
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

		this.clock_update_interval_id = setInterval(this.clockUpdate.bind(this), 1000);
		this.weather_update_interval_id = setInterval(this.weatherUpdate.bind(this), 1000 * 60 * 10 + 1000);
	}

	componentDidMount() {
		this.weather_audio = new Audio();
		window.audio = new Audio();
		this.weatherUpdate();

		//Get music set
		const url = window.location.origin + "/api/get_set" + "?" + 
			"access_key=" + this.access_key;

		fetch(url)
			.then(function(data){
				return data.json();
			})
			.then((json) =>{
				this.enabled_sets = json
			});
	 }  

	pad(num) {
		var result = num + "";
		while(result.length < 2) {
			result = "0" + result;
		}
		return result;
	}

	makeID(length) {
		var result           = '';
		var characters       = 'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789';
		var charactersLength = characters.length;
		for ( var i = 0; i < length; i++ ) {
		   result += characters.charAt(Math.floor(Math.random() * charactersLength));
		}
		return result;
	}
	

	playWeatherEffect() {
		const weather_audio_src = window.location.origin + "/api/get_weather_effect/" + this.makeID(20) + "?" + 
			"access_key=" + this.access_key + "&" +
			"city_name=" +  this.city + "&" +
			"country_code=" + this.country_code;
		
		this.weather_audio.pause();
		this.weather_audio.src = weather_audio_src;
		this.weather_audio.play();
	}

	weatherUpdate() {
		if(this.state.playing) {
			this.playWeatherEffect();
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

	getCurrentTimeString() {
		const now = new Date();
		return this.pad(now.getHours()) + ":" + this.pad(now.getMinutes()) + ":" + this.pad(now.getSeconds());
	}

	clockUpdate() {
		const background = this.getBackground();
		document.body.style = 'background: ' + background + ';';
		
		if(background) {
			this.setState({
				current_time: this.getCurrentTimeString(),
				timer_text_color: this.getContrastYIQ(background)
			});
		}
	}

	getMin(now) {
		if(now.getMinutes() < 10) {
			return "0";
		}

		return (now.getMinutes() + "")[0]
	}

	rand(seed) {
		var t = seed += 0x6D2B79F5;
		t = Math.imul(t ^ t >>> 15, t | 1);
		t ^= t + Math.imul(t ^ t >>> 7, t | 61);
		return ((t ^ t >>> 14) >>> 0) / 4294967296;
	}

	getRandomGame() {
		while(this.enabled_sets.length == 0) {
		}

		const now = new Date();
		var x = 0;
		if(now.getMinutes() % 10 >= 5){
			x = 0
		} else {
			x = 5
		}
		var seed = parseInt(x + this.getMin(now) + now.getDay() + now.getMonth() + now.getFullYear());
		
		console.log("Seed: " + seed);

		var max = this.enabled_sets.length;
		var min = 0;
		var num = this.rand(seed) * 1000000000;
		console.log("Random Number: " + num);
		var idx = Math.floor(num % (max - min) + min);
		
		return this.enabled_sets[idx];
	}

	startVibing = () => {
		const now = new Date();

		this.playWeatherEffect();

		window.audio.volume	= this.state.volume / 100;

		var hour = now.getHours()
		
		if(this.state.wacky) {
			if(hour >= 12) {
				hour = hour - 12
			} else {
				hour = hour + 12
			}
		}

		var game = this.getRandomGame()
		console.log(game)

		const next_src = window.location.origin + "/api/get_sample/" + game + "/" + hour + "?" + 
			"access_key=" + this.access_key + "&" +
			"city_name=" +  this.city + "&" +
			"country_code=" + this.country_code;

		window.audio.pause();
		window.audio.src = next_src;
		window.audio.onloadedmetadata = function() {
			var x = now.getMinutes() % 10;
			if(x >= 5){
				x -= 5;
			}
			x = x / 5;
			x = ((window.audio.duration * x) + now.getSeconds());
			console.log("time " + x);
			console.log("duration " + window.audio.duration);
			window.audio.currentTime = x;
			window.audio.play();
		};

		console.log("Playing next song! hour:" + hour + " wacky:" + this.state.wacky);

		this.setState({
			hour: now.getHours()
		});
	}

	musicUpdate = () => {
	
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
			this.startVibing();
			this.bell_played = false
		}
	}

	initAudio = () => {
		this.play_sound_interval_id = setInterval(this.musicUpdate.bind(this), 1000);

		this.setState({
			playing: true
		});

		this.startVibing();
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
	  
	getColor(now, start_time, end_time, start_color, middle_color, end_color) {
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

	getBackground() {
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
			result = this.getColor(now, times.nightEnd, times.dawn, sky_night, sky_dawnset, sky_dawn);
		} else if(now > times.dawn && now < times.sunriseEnd) {
			// Sunrise 
			result = this.getColor(now, times.dawn, times.sunriseEnd, sky_dawn, sky_sunrise, sky_day);
		} else if(now > times.sunriseEnd && now < times.sunsetStart) {
			// Daytime
			result = sky_day;
		} else if(now > times.sunsetStart && now < times.dusk) {
			// Sunset
			result = this.getColor(now, times.sunsetStart, times.dusk, sky_day, sky_sunset, sky_dusk);
		} else if(now > times.dusk && now < times.night) {
			// Dusk
			result = this.getColor(now, times.dusk, times.night, sky_dusk, sky_duskset, sky_night);
		} else {
			// Nighttime
			result = sky_night;
		}

		if(this.state.wacky){
			result = invert(result);
		}

		return result;
	}

	showLoading() {
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

	renderToggleButton = () => {
		return <div className="text-center">
			<ToggleButton
				value="check"
				selected={this.state.wacky}
				onChange={() => {
					this.setState({
						wacky: !this.state.wacky
					}, () => {
						console.log("wacky: " + this.state.wacky)
						this.startVibing()
					});
				}}
				>
				<CheckIcon />
			</ToggleButton>
		</div>

	}

	showContent() {
		let { volume } = this.state;

		return (
			<>
				<Button variant="outline-danger" onClick={this.state.playing ? this.pause_song : this.initAudio} disabled={this.state.playing}>
					<img src={this.get_house()} alt="Italian Trulli" />
					{this.state.playing ? <div>PLAYING</div> : <div>Not playing</div>}
				</Button>
				<Slider
					value={volume}
					orientation="horizontal"
					onChange={this.handleOnChange}
				/>
				{ this.state.playing ? this.renderToggleButton() : <></> }
			</>
		);
	}

	showError() {
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
					{this.state.loading ? this.showLoading() : (this.state.error ? this.showError() : this.showContent()) }
				</div>
			</div>
		)
	}
}
  
export default MusicPlayer;