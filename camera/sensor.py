"""OV5647 sensor tuning for Rock 5A (V4L2 subdev).

Manual exposure/gain avoids blown-out highlights on the ball under bright
overhead lighting. Values can be overridden via threshold.json or env vars.
"""

import os
import subprocess

from camera import debug

DEFAULT_SUBDEV = "/dev/v4l-subdev2"
DEFAULT_EXPOSURE = 900
DEFAULT_GAIN = 80


def _int_setting(settings, key, env_key, default):
    if settings and key in settings:
        return int(settings[key])
    if env_key in os.environ:
        return int(os.environ[env_key])
    return default


def configure_sensor(settings=None):
    """Applies OV5647 exposure/gain before opening /dev/video11."""
    settings = settings or {}

    subdev = settings.get("cameraSensorSubdev") or os.environ.get(
        "CAMERA_SENSOR_SUBDEV", DEFAULT_SUBDEV
    )
    auto_exposure = str(
        settings.get(
            "cameraAutoExposure",
            os.environ.get("CAMERA_AUTO_EXPOSURE", "0"),
        )
    ).lower() in ("1", "true", "yes")

    exposure = _int_setting(settings, "cameraExposure", "CAMERA_EXPOSURE", DEFAULT_EXPOSURE)
    gain = _int_setting(settings, "cameraGain", "CAMERA_GAIN", DEFAULT_GAIN)
    exposure = max(4, min(exposure, 1964))
    gain = max(16, min(gain, 1023))

    if auto_exposure:
        ctrl = "auto_exposure=0,gain_automatic=1"
    else:
        ctrl = f"auto_exposure=1,gain_automatic=0,exposure={exposure},analogue_gain={gain}"

    result = subprocess.run(
        ["v4l2-ctl", "-d", subdev, f"--set-ctrl={ctrl}"],
        capture_output=True,
        text=True,
    )
    if result.returncode != 0:
        debug.log(f"Warning: failed to set sensor controls on {subdev}: {result.stderr.strip()}")
        return False

    debug.log(f"Sensor configured on {subdev}: {ctrl}")
    return True
