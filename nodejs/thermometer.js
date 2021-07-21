#!/usr/bin/node

const noble = require('@abandonware/noble');
const LRU = require('lru-cache');

const SERVICE_UUID = '181a';
const ServiceDataType = Uint8Array;
const BITS_PER_ELEMENT = ServiceDataType.BYTES_PER_ELEMENT * 8;

const ADDRESS_TO_NAME = {
  'a4:c1:38:59:00:11': 'Dnevna',
  'a4:c1:38:15:e4:e7': 'Lođa'
}

const getTime =
  () =>
    new Date()
      .toLocaleString('hr-HR')
  ;

const write =
  (...text) => {
    const out = text.join('');
    process.stdout.write(out);
    return out;
  }
  ;

const code =
  (...text) =>
    '\033[' + text.join('')
  ;

const moveUp =
  (lines) =>
    code(`${lines}A`) + '\r'
  ;

const clearLine =
  () =>
    code('K')
  ;

const log =
  (text) =>
    write(clearLine(), text, '\r')
  ;

const logAndReturn =
  (text) => {
    const linesWritten = text.split('\n').length;
    log(text);
    write(moveUp(linesWritten - 1));
  }
  ;

const thermometers = new Map();
const foundDevices = new LRU(20);

const updateFoundDevice =
  (
    {
      address,
      advertisement,
    },
  ) => {
    foundDevices.set(
      address,
      {
        address,
        localName: advertisement.localName,
        time: getTime(),
      },
      60_000,
    );
  }
  ;

const updateThermometer =
  (
    {
      address,
      advertisement,
    },
  ) => {
    const service =
      advertisement
        .serviceData
        .find(
          ({ uuid }) =>
            String(uuid).toLowerCase() === SERVICE_UUID.toLowerCase()
          ,
        )
      ;

    if (!service) {
      return;
    }

    // https://github.com/atc1441/ATC_MiThermometer#advertising-format-of-the-custom-firmware
    // Service (id: 181A) data description:
    // MAC:   1-6
    // TEMP:  7-8
    // HUMI:  9
    // BATT%: 10
    // BATTv: 11-12
    const d = new ServiceDataType(service.data);

    const temperature = (d[6] << BITS_PER_ELEMENT | d[7]) / 10;
    const humidity = d[8];
    const battery = d[9];

    const data = {
      address,
      temperature,
      humidity,
      battery,
      time: getTime(),
      d,
    };

    thermometers.set(
      address,
      data,
    );
  }
  ;

async function main() {
  console.clear();
  log('|> SCAN: STARTING...');

  await noble.startScanningAsync([], true);
  log('|> SCAN: STARTED\n');

  noble.on('discover', async (peripheral) => {
    const {
      address,
    } = peripheral;

    if (address.startsWith('a4:c1:38:')) {
      updateThermometer(peripheral);
    } else {
      updateFoundDevice(peripheral);
    }

    console.clear();
    log(getTime() + '\n');
    log('\n');

    if (thermometers.size <= 0) {
      log('Found:\n');

      let n = 0;
      foundDevices.forEach(({ address, localName, time }) => {
        log(` - ${address} (${localName}) [${time}]\n`);
        n += 1;
      });

      return;
    }

    const formatEntry =
      ({ address, temperature, humidity, battery, time }) =>
        `
|->     Address: ${ADDRESS_TO_NAME[address] || address}
|->          At: ${time}
|-> Temperature: ${temperature}°C
|->    Humidity: ${humidity}%
|->     Battery: ${battery}%
  `.trim()
      ;

    const toWrite =
      Array
        .from(thermometers.values())
        .map(formatEntry)
      ;

    log(toWrite.join('\n--------------------\n') + '\n');
  });
}

main();
