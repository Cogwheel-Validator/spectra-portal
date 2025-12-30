import bunyan from 'bunyan';

const logger = bunyan.createLogger({name: 'spectra-ibc-hub-app'});

export default logger;