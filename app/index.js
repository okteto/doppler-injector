const fs = require('fs');
const readline = require('readline');
const backup = '/var/secret/doppler/.doppler'
const result = '/var/secret/doppler/.env'

try{
  require("doppler-client")({
      api_key: process.env.DOPPLER_API_KEY,
      pipeline: process.env.DOPPLER_PIPELINE,
      environment: process.env.DOPPLER_ENVIRONMENT,
      backup_filepath: backup
    })
} catch (e) {
  console.error(`doppler api call failed, will use cache if available: ${e}`)
}

fs.access(backup, fs.F_OK, (err) => {
  if (err) {
    console.error(`${backup} doesn't exist`)
    process.exit(1)
  }
})

const readFile = readline.createInterface({
    input: fs.createReadStream(backup),
    output: fs.createWriteStream(result, {flags:'w'}),
    terminal: false
});

readFile
.on('line', transform)
.on('close', function() {console.log(`Created "${this.output.path}"`);});

function transform(line) {
    var arr = line.split("=").map(val => val.trim())
    this.output.write(`export ${arr[0]}=${arr[1]}\n`);
}