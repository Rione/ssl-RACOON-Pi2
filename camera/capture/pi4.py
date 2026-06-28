"""Raspberry Pi 4B capture backend using Picamera2.

Mirrors the original main.py behaviour: Picamera2 is configured with the
"RGB888" preview format, and the captured array is consumed directly by the
downstream HSV pipeline (which treats it as BGR, as the original code did).
"""

from picamera2 import Picamera2


class Pi4Capture:
    def __init__(self, settings=None):
        if settings is None:
            settings = {}

        self.cap = Picamera2()
        config = self.cap.create_preview_configuration({"format": "RGB888"})
        self.cap.configure(config)
        self.cap.start()

        self._width = int(settings.get("frameWidth", 640))
        self._height = int(settings.get("frameHeight", 480))
        print(f"Pi4 (Picamera2) capture started: target {self._width}x{self._height}")

    def read(self):
        return (True, self.cap.capture_array())

    def release(self):
        self.cap.close()
        print("Pi4 video capture released.")
