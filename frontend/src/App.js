import React from 'react';
import MusicPlayer from './MusicPlayer';
import LoginScreen from './LoginScreen';
import {
	BrowserRouter as Router,
	Switch,
	Route,
  } from "react-router-dom";
import { Helmet } from 'react-helmet'
import './App.css';

import 'bootstrap/dist/css/bootstrap.min.css';

class App extends React.Component {
	constructor(props) {
		super(props);
	}

	componentDidMount() {
		document.title = "Vibes"
	}

	render() {
		return (
			<div>
				<Router>
					<div>
						<Switch>
							<Route path="/login">
								<LoginScreen />
							</Route>
							<Route path="/">
								<MusicPlayer />
							</Route>
						</Switch>
					</div>
				</Router>
			</div>
		)
	}
}
  
export default App;
