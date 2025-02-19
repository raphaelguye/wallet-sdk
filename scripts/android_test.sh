#!/bin/bash
#
# Copyright Avast Software. All Rights Reserved.
#
# SPDX-License-Identifier: Apache-2.0
#

cd test/integration
INITIATE_ISSUANCE_URL="$(../../build/bin/integration-cli issuance bank_issuer)"
INITIATE_VERIFICATION_URL="$(../../build/bin/integration-cli verification v_myprofile_jwt_verified_employee)"

echo "INITIATE_ISSUANCE_URL:${INITIATE_ISSUANCE_URL}"
echo "INITIATE_VERIFICATION_URL:${INITIATE_VERIFICATION_URL}"


cd ../../demo/app/android
adb reverse tcp:8075 tcp:8075 && adb reverse tcp:8072 tcp:8072 &&  adb reverse tcp:9229 tcp:9229 && \
  ./gradlew app:connectedAndroidTest -PINITIATE_ISSUANCE_URL="${INITIATE_ISSUANCE_URL}" -PINITIATE_VERIFICATION_URL="${INITIATE_VERIFICATION_URL}"