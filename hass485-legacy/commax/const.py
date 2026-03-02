"""Constants for the Commax integration."""

DOMAIN = "commax"

# USB to RS485 Device Types
DEVICE_TYPE_LIGHT = "light"
DEVICE_TYPE_BOILER = "boiler"
DEVICE_TYPE_DOOR = "door"
DEVICE_TYPE_ELEVATOR = "elevator"

# Device Counts
LIGHT_COUNT = 5
BOILER_COUNT = 4
DOOR_BUTTON_COUNT = 1  # 현관문 열기 버튼
DOORBELL_COUNT = 1
ELEVATOR_BUTTON_COUNT = 1  # 엘리베이터 호출 버튼
MASTER_SWITCH_COUNT = 1  # 일괄소등 스위치

# Default Names
DEFAULT_NAME = "Commax"
DEFAULT_LIGHT_DEVICE = "/dev/ttyUSB0"
DEFAULT_BOILER_DEVICE = "/dev/ttyUSB1"
DEFAULT_DOOR_DEVICE = "/dev/ttyUSB2"
DEFAULT_ELEVATOR_DEVICE = "/dev/ttyUSB3"

# Configuration Keys
CONF_LIGHT_DEVICE = "light_device"
CONF_BOILER_DEVICE = "boiler_device"
CONF_DOOR_DEVICE = "door_device"
CONF_ELEVATOR_DEVICE = "elevator_device" 