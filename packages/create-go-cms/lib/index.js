const { scaffoldProject } = require('./scaffolder');
const { getPlatformInfo } = require('./platform');
const { ensureBinary } = require('./downloader');

module.exports = {
  scaffoldProject,
  getPlatformInfo,
  ensureBinary
};

