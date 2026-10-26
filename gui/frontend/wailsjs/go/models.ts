export namespace downloader {
	
	export class DubVersion {
	    audio_locale: string;
	    guid: string;
	
	    static createFrom(source: any = {}) {
	        return new DubVersion(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.audio_locale = source["audio_locale"];
	        this.guid = source["guid"];
	    }
	}
	export class SeasonSummary {
	    id: string;
	    title: string;
	    number: number;
	    episodeCount: number;
	    audioLocales: string[];
	
	    static createFrom(source: any = {}) {
	        return new SeasonSummary(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.title = source["title"];
	        this.number = source["number"];
	        this.episodeCount = source["episodeCount"];
	        this.audioLocales = source["audioLocales"];
	    }
	}
	export class Title {
	    id: string;
	    seriesTitle: string;
	    title: string;
	    season: number;
	    number: number;
	    thumbnail: string;
	    availabilityStarts: string;
	    audioLocale: string;
	    versions: DubVersion[];
	
	    static createFrom(source: any = {}) {
	        return new Title(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.seriesTitle = source["seriesTitle"];
	        this.title = source["title"];
	        this.season = source["season"];
	        this.number = source["number"];
	        this.thumbnail = source["thumbnail"];
	        this.availabilityStarts = source["availabilityStarts"];
	        this.audioLocale = source["audioLocale"];
	        this.versions = this.convertValues(source["versions"], DubVersion);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class LookupResult {
	    kind: string;
	    seriesTitle: string;
	    episode?: Title;
	    seasons?: SeasonSummary[];
	
	    static createFrom(source: any = {}) {
	        return new LookupResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.kind = source["kind"];
	        this.seriesTitle = source["seriesTitle"];
	        this.episode = this.convertValues(source["episode"], Title);
	        this.seasons = this.convertValues(source["seasons"], SeasonSummary);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class Options {
	    audioLangs: string;
	    subsLangs: string;
	    ccLangs: string;
	    videoQuality: string;
	    audioQuality: string;
	    outputDir: string;
	    simultaneous: number;
	    downloadDelay: number;
	    debugManifest: boolean;
	    widevinePath: string;
	
	    static createFrom(source: any = {}) {
	        return new Options(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.audioLangs = source["audioLangs"];
	        this.subsLangs = source["subsLangs"];
	        this.ccLangs = source["ccLangs"];
	        this.videoQuality = source["videoQuality"];
	        this.audioQuality = source["audioQuality"];
	        this.outputDir = source["outputDir"];
	        this.simultaneous = source["simultaneous"];
	        this.downloadDelay = source["downloadDelay"];
	        this.debugManifest = source["debugManifest"];
	        this.widevinePath = source["widevinePath"];
	    }
	}
	

}

export namespace main {
	
	export class Environment {
	    ffmpegPath: string;
	    ffmpegError?: string;
	    widevine: boolean;
	    loggedIn: boolean;
	    loginError?: string;
	
	    static createFrom(source: any = {}) {
	        return new Environment(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ffmpegPath = source["ffmpegPath"];
	        this.ffmpegError = source["ffmpegError"];
	        this.widevine = source["widevine"];
	        this.loggedIn = source["loggedIn"];
	        this.loginError = source["loginError"];
	    }
	}
	export class Track {
	    id: number;
	    title: string;
	    label: string;
	    current: number;
	    total: number;
	    rateMbps: number;
	
	    static createFrom(source: any = {}) {
	        return new Track(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.title = source["title"];
	        this.label = source["label"];
	        this.current = source["current"];
	        this.total = source["total"];
	        this.rateMbps = source["rateMbps"];
	    }
	}
	export class Job {
	    id: string;
	    title: downloader.Title;
	    status: string;
	    error?: string;
	    output?: string;
	    // Go type: time
	    retryAt?: any;
	    tracks: Track[];
	
	    static createFrom(source: any = {}) {
	        return new Job(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.title = this.convertValues(source["title"], downloader.Title);
	        this.status = source["status"];
	        this.error = source["error"];
	        this.output = source["output"];
	        this.retryAt = this.convertValues(source["retryAt"], null);
	        this.tracks = this.convertValues(source["tracks"], Track);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class Language {
	    code: string;
	    name: string;
	
	    static createFrom(source: any = {}) {
	        return new Language(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.code = source["code"];
	        this.name = source["name"];
	    }
	}
	export class LogLine {
	    time: string;
	    text: string;
	
	    static createFrom(source: any = {}) {
	        return new LogLine(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.time = source["time"];
	        this.text = source["text"];
	    }
	}
	export class Settings {
	    etpRt: string;
	    ffmpegPath: string;
	    options: downloader.Options;
	
	    static createFrom(source: any = {}) {
	        return new Settings(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.etpRt = source["etpRt"];
	        this.ffmpegPath = source["ffmpegPath"];
	        this.options = this.convertValues(source["options"], downloader.Options);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

