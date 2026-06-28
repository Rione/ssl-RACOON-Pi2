"""Encodes detection results into the JSON payload consumed by the Go side.

The schema (frame/x/y/isball) matches internal/state/state.go ImageData.
"""

import base64
import json

import cv2


class Encoder:
    @staticmethod
    def encodeData(frame, center=None, distance=None, quality=90):
        if frame is None or frame.size == 0:
            print("Error: Cannot encode empty frame.")
            return None

        encode_param = [int(cv2.IMWRITE_JPEG_QUALITY), quality]
        result, encoded_image = cv2.imencode(".jpg", frame, encode_param)

        if not result:
            print("Error: Failed to encode image to JPEG.")
            return None

        frame_bytes_b64 = base64.b64encode(encoded_image.tobytes()).decode("utf-8")

        x_coord = center[0] if center is not None else None
        y_coord = distance if center is not None else None
        isball = center is not None

        data = {"frame": frame_bytes_b64, "x": x_coord, "y": y_coord, "isball": isball}

        try:
            return json.dumps(data)
        except TypeError as e:
            print(f"Error serializing data to JSON: {e}")
            return None
