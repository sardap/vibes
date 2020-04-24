import wtforms_json

from dateutil import tz
from wtforms import Form, StringField, BooleanField, IntegerField, Field
from wtforms.validators import DataRequired, Regexp, ValidationError, NumberRange

ENABLED_GAMES = ["new_leaf", "wild_world", "city_folk", "gamecube", "new_horizons"]

def validate_enabled_games(form, field):
	try:
		games = field.data.split(",")
		
		if any(game not in ENABLED_GAMES for game in games):
			raise ValidationError('invalid list')
	except:
		raise ValidationError('invalid list')

class TimeZoneField(StringField):
	def process_formdata(self, valuelist):
		if(len(valuelist) != 1):
			raise ValidationError('Invalid Input')
		
		self.data = tz.gettz(valuelist[0])

		if(self.data == None):
			raise ValidationError('Invalid Input')


class CreateSampleForm(Form):
	timestamp = IntegerField(
		'timestamp',
		[
			DataRequired()
		]
	)
	tz = TimeZoneField(
		"tz",
		[
			DataRequired()
		]
	)
	enabled_games = StringField(
		'enabled_games',
		[
			DataRequired(),
			validate_enabled_games
		]
	)
	city_name = StringField(
		'city',
		[
			DataRequired()
		]
	)
	country_code = StringField(
		'country_code',
		[
			DataRequired()
		]
	)
	access_key = StringField(
		'access_key',
		[
			DataRequired()
		]
	)

class LoginForm(Form):
	city_name = StringField(
		'city_name',
		[
			DataRequired()
		]
	)
	country_code = StringField(
		'country_code',
		[
			DataRequired()
		]
	)
	access_key = StringField(
		'access_key',
		[
			DataRequired()
		]
	)

class GetSampleForm(Form):
	access_key = StringField(
		'access_key',
		[
			DataRequired()
		]
	)
	city_name = StringField(
		'city',
		[
			DataRequired()
		]
	)
	country_code = StringField(
		'country_code',
		[
			DataRequired()
		]
	)
