"""Rock5A capture backend using OpenCV V4L2.

On the Radxa Rock5A (RK3588) the MIPI CSI camera is exposed as ``/dev/video11``
by default, so OpenCV opens device index 11. ``cv2.VideoCapture`` already
returns BGR frames, matching the downstream HSV pipeline.

The device index can be overridden via the ``cameraDevice`` key in
threshold.json (useful for USB cameras at /dev/video0, etc.).
"""

import cv2

DEFAULT_DEVICE = 11


class Rock5aCapture:
    def __init__(self, settings=None):
        if settings is None:
            settings = {}

        device = int(settings.get("cameraDevice", DEFAULT_DEVICE))
        self._device = device
        self.cap = cv2.VideoCapture(device)
        if not self.cap.isOpened():
            raise IOError(f"Cannot open camera device {device} (V4L2)")

        width = int(settings.get("frameWidth", 640))
        height = int(settings.get("frameHeight", 480))
        fps = int(settings.get("fps", 30))
        buffer_size = int(settings.get("bufferSize", 4))

        self.cap.set(cv2.CAP_PROP_FRAME_WIDTH, width)
        self.cap.set(cv2.CAP_PROP_FRAME_HEIGHT, height)
        self.cap.set(cv2.CAP_PROP_FPS, fps)
        try:
            self.cap.set(cv2.CAP_PROP_BUFFERSIZE, buffer_size)
        except Exception:
            pass

        print(
            f"Rock5A (V4L2) capture started on /dev/video{device}: "
            f"target {width}x{height} @ {fps}fps"
        )

    def read(self):
        return self.cap.read()

    def release(self):
        if self.cap is not None:
            self.cap.release()
        print("Rock5A video capture released.")
