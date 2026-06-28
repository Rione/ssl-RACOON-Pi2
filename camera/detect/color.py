"""HSV color + contour based ball detection.

Moved from the original main.py. ``ImageProcessor`` and ``BallDetector`` support
hot-reloading their thresholds via ``update_settings`` so calibration can take
effect without restarting the process.
"""

import cv2
import numpy as np


class ImageProcessor:
    def __init__(self, settings=None):
        if settings is None:
            settings = {}

        self._minThreshold = settings.get(
            "minThreshold", np.array([1, 120, 100], dtype=np.uint8)
        )
        self._maxThreshold = settings.get(
            "maxThreshold", np.array([15, 255, 255], dtype=np.uint8)
        )
        self._ksize = tuple(settings.get("gaussianKernelSize", (5, 5)))
        self._sigmaX = settings.get("gaussianSigmaX", 0)
        self._shape = cv2.MORPH_RECT
        self._size = tuple(settings.get("morphKernelSize", (3, 3)))
        self._operation = cv2.MORPH_OPEN

    def update_thresholds(self, min_threshold, max_threshold):
        self._minThreshold = min_threshold
        self._maxThreshold = max_threshold

    def extractColors(self, frame):
        filtered = self._filterFrame(frame)
        hsv = cv2.cvtColor(filtered, cv2.COLOR_BGR2HSV)
        hsv = self._equalizeHist(hsv)
        mask = cv2.inRange(hsv, self._minThreshold, self._maxThreshold)
        mask = self._applyMorphologicalTransformations(mask)
        return mask

    def _filterFrame(self, frame):
        return cv2.GaussianBlur(frame, self._ksize, self._sigmaX)

    def _equalizeHist(self, hsv):
        h, s, v = cv2.split(hsv)
        clahe = cv2.createCLAHE(clipLimit=2.0, tileGridSize=(8, 8))
        v_equalized = clahe.apply(v)
        return cv2.merge((h, s, v_equalized))

    def _applyMorphologicalTransformations(self, mask):
        kernel = cv2.getStructuringElement(self._shape, self._size)
        return cv2.morphologyEx(mask, self._operation, kernel)


class BallDetector:
    def __init__(self, settings=None):
        if settings is None:
            settings = {}

        self.imageProcessor = ImageProcessor(settings)
        self._radius = int(settings.get("ballDetectRadius", 150))
        self._circularityThreshold = float(settings.get("circularityThreshold", 0.2))
        self._minContourArea = float(settings.get("minContourArea", 100))
        self._previousCenter = None

    def update_settings(self, settings):
        """Hot-reloads thresholds after a calibration run."""
        self.imageProcessor.update_thresholds(
            settings.get("minThreshold", self.imageProcessor._minThreshold),
            settings.get("maxThreshold", self.imageProcessor._maxThreshold),
        )
        if "ballDetectRadius" in settings:
            self._radius = int(settings["ballDetectRadius"])
        if "circularityThreshold" in settings:
            self._circularityThreshold = float(settings["circularityThreshold"])
        self._previousCenter = None

    def detect(self, frame):
        roi, offset, vertices = self._focus(frame, self._previousCenter)
        if roi.size == 0:
            print("Warning: ROI is empty.")
            self._previousCenter = None
            return None, None, None, None

        mask = self.imageProcessor.extractColors(roi)
        contours, _ = cv2.findContours(mask, cv2.RETR_EXTERNAL, cv2.CHAIN_APPROX_SIMPLE)

        valid_contours = [
            cnt for cnt in contours if cv2.contourArea(cnt) > self._minContourArea
        ]

        if not valid_contours:
            return None, None, vertices, None

        bestContour = max(valid_contours, key=cv2.contourArea)

        if self._isCircular(bestContour):
            (x, y), radius = cv2.minEnclosingCircle(bestContour)
            center = (int(x + offset[0]), int(y + offset[1]))
            circleContour = self._createCircleContour(
                x + offset[0], y + offset[1], radius
            )
            self._previousCenter = center
            diameter = int(radius * 2)
            distance = 320 * 40 / diameter if diameter > 0 else None
            return center, circleContour, vertices, distance

        self._previousCenter = None
        return None, None, vertices, None

    def _isCircular(self, contour):
        perimeter = cv2.arcLength(contour, True)
        area = cv2.contourArea(contour)
        if perimeter == 0 or area == 0:
            return False
        circularity = (4 * np.pi * area) / (perimeter**2)
        return circularity > self._circularityThreshold

    def _focus(self, frame, center):
        height, width = frame.shape[:2]
        radius = self._radius

        if center is not None:
            cx, cy = center
            xMin = max(0, cx - radius)
            yMin = max(0, cy - radius)
            xMax = min(width, cx + radius)
            yMax = min(height, cy + radius)
        else:
            xMin, yMin, xMax, yMax = 0, 0, width, height

        xMin, yMin, xMax, yMax = int(xMin), int(yMin), int(xMax), int(yMax)

        if yMin >= yMax or xMin >= xMax:
            print(
                f"Warning: Invalid ROI calculated: ({xMin},{yMin}) to ({xMax},{yMax}). Using full frame."
            )
            xMin, yMin, xMax, yMax = 0, 0, width, height

        roi = frame[yMin:yMax, xMin:xMax]
        offset = (xMin, yMin)
        vertices = (xMin, yMin, xMax, yMax)
        return roi, offset, vertices

    def _createCircleContour(self, centerX, centerY, radius, num_points=36):
        angles = np.linspace(0, 2 * np.pi, num_points, endpoint=False)
        contour_points = np.array(
            [
                [centerX + radius * np.cos(ang), centerY + radius * np.sin(ang)]
                for ang in angles
            ],
            dtype=np.int32,
        )
        return contour_points.reshape((-1, 1, 2))


class Visualizer:
    def __init__(self, radius=5, windowName="Frame"):
        self._radius = radius
        self._windowName = windowName

    def draw(self, frame, center, circleContour, vertices):
        if vertices:
            cv2.rectangle(
                frame,
                (vertices[0], vertices[1]),
                (vertices[2], vertices[3]),
                (0, 0, 255),
                1,
            )

        if circleContour is not None:
            cv2.drawContours(frame, [circleContour], -1, (255, 0, 0), 2)

        if center is not None:
            cv2.circle(frame, center, self._radius, (0, 255, 0), -1)

        return frame

    def destroy(self):
        pass
