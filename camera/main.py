"""Camera main loop with on-demand YOLO color calibration.

Normal operation runs only the lightweight HSV + contour detector and publishes
results over UDP. A background calibration server shares the same camera and,
when triggered, runs YOLO once to recompute HSV thresholds.
"""

import threading
import time

import cv2

from camera.calib_server import CalibServer
from camera.capture.factory import create_capture
from camera.detect.calib import calibrate
from camera.detect.color import BallDetector, Visualizer
from camera.settings import load_settings, save_thresholds, threshold_to_string
from camera.transport.encoder import Encoder
from camera.transport.udp_client import UDPClient


class CameraContext:
    """Shared, lock-guarded state between the main loop and calib server."""

    def __init__(self, capture, detector, settings):
        self.capture = capture
        self.detector = detector
        self.settings = settings
        self.lock = threading.Lock()

    def read(self):
        with self.lock:
            return self.capture.read()

    def run_calibration(self):
        """Captures a frame, calibrates, persists, and hot-reloads the detector.

        Returns a JSON-serializable dict for the calib server to send back.
        """
        with self.lock:
            ret, frame = self.capture.read()

        if not ret or frame is None:
            return {"ok": False, "error": "failed to capture frame"}

        result = calibrate(frame, self.settings)
        if not result.get("ok"):
            return {"ok": False, "error": result.get("error", "calibration failed")}

        min_threshold = result["minThreshold"]
        max_threshold = result["maxThreshold"]
        ball_detect_radius = result["ballDetectRadius"]
        circularity_threshold = float(self.settings.get("circularityThreshold", 0.2))

        save_thresholds(
            min_threshold, max_threshold, ball_detect_radius, circularity_threshold
        )

        # Update parsed settings and hot-reload the detector under the lock.
        self.settings["minThreshold"] = min_threshold
        self.settings["maxThreshold"] = max_threshold
        self.settings["ballDetectRadius"] = ball_detect_radius
        with self.lock:
            self.detector.update_settings(self.settings)

        return {
            "ok": True,
            "minThreshold": threshold_to_string(min_threshold),
            "maxThreshold": threshold_to_string(max_threshold),
            "ballDetectRadius": int(ball_detect_radius),
            "bbox": result["bbox"],
            "confidence": result.get("confidence"),
            "samplePoints": result["samplePoints"],
            "previewFrame": result.get("previewFrame"),
        }


def main():
    settings = load_settings()

    capture = None
    udpClient = None
    visualizer = None

    try:
        ballDetector = BallDetector(settings)
        visualizer = Visualizer()
        capture = create_capture(settings)
        udpClient = UDPClient(port=int(settings.get("udpPort", 31133)))

        context = CameraContext(capture, ballDetector, settings)

        calib_server = CalibServer(context.run_calibration)
        calib_server.start()

        output_width = int(settings.get("outputFrameWidth", 160))
        output_height = int(settings.get("outputFrameHeight", 96))
        jpeg_quality = int(settings.get("jpegQuality", 90))

        while True:
            ret, frame = context.read()
            if not ret or frame is None:
                print("Error: Failed to capture frame from camera.")
                time.sleep(0.5)
                continue

            center, circleContour, vertices, distance = ballDetector.detect(
                frame.copy()
            )

            print(center, distance)

            frame = visualizer.draw(frame, center, circleContour, vertices)

            frame_resized = cv2.resize(
                frame, (output_width, output_height), interpolation=cv2.INTER_AREA
            )

            encoded_json_string = Encoder.encodeData(
                frame_resized, center, distance, quality=jpeg_quality
            )

            if encoded_json_string:
                udpClient.send(encoded_json_string)

    except IOError as e:
        print(f"Initialization Error: {e}")
    except KeyboardInterrupt:
        print("Program interrupted by user.")
    except Exception as e:
        print(f"An unexpected error occurred in main loop: {e}")
        import traceback

        traceback.print_exc()
    finally:
        print("Cleaning up resources...")
        if capture is not None:
            capture.release()
        if udpClient is not None:
            udpClient.close()
        if visualizer is not None:
            visualizer.destroy()
        print("Cleanup complete.")


if __name__ == "__main__":
    main()
