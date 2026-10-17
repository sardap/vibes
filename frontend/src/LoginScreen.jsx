import React from 'react';
import Form from 'react-bootstrap/Form';
import Button from 'react-bootstrap/Button';
import './App.css'

import 'bootstrap/dist/css/bootstrap.min.css';

class LoginScreen extends React.Component {
	constructor(props) {
		super(props);
	}

	show_form() {
		return (
			<div className="centered text-center">
				<form action="/api/login" method="post">
					<Form.Group>
						<Form.Label>Access Key</Form.Label>
						<Form.Control type="username" placeholder="Enter access key" name="access_key" for="access_key" />
					</Form.Group>
					<Form.Group>
						<Form.Label>Country Code</Form.Label>
						<Form.Control type="text" placeholder="AU" name="country_code" for="country_code" />
					</Form.Group>
					<Form.Group>
						<Form.Label>City</Form.Label>
						<Form.Control type="text" placeholder="City" name="city_name" for="city_name" />
					</Form.Group>
					<Button variant="primary" type="submit" block>
						Submit
					</Button>
				</form>
			</div>
		);
	}

	render() {
		return (
			this.show_form()
		)
	}
}
  
export default LoginScreen;