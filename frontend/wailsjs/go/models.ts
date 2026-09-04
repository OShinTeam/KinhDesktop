export namespace global {
	
	export class LanguageInfo {
	    language_name: string;
	    language_code: string;
	    textmap_path: string;
	    translation_progress: string;
	    translator: string;
	    last_updated: string;
	    version: string;
	
	    static createFrom(source: any = {}) {
	        return new LanguageInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.language_name = source["language_name"];
	        this.language_code = source["language_code"];
	        this.textmap_path = source["textmap_path"];
	        this.translation_progress = source["translation_progress"];
	        this.translator = source["translator"];
	        this.last_updated = source["last_updated"];
	        this.version = source["version"];
	    }
	}
	export class LanguagePack {
	    language_name: string;
	    language_code: string;
	    textmap_path: string;
	    translation_progress: string;
	    translator: string;
	    last_updated: string;
	    version: string;
	    textmap: Record<string, string>;
	
	    static createFrom(source: any = {}) {
	        return new LanguagePack(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.language_name = source["language_name"];
	        this.language_code = source["language_code"];
	        this.textmap_path = source["textmap_path"];
	        this.translation_progress = source["translation_progress"];
	        this.translator = source["translator"];
	        this.last_updated = source["last_updated"];
	        this.version = source["version"];
	        this.textmap = source["textmap"];
	    }
	}

}

export namespace service {
	
	export class BaiduLoginResult {
	    success: boolean;
	    username: string;
	    user_id: string;
	    bduss: string;
	    ptoken: string;
	    stoken: string;
	    bd_stoken: string;
	    vip_type: number;
	    photo_url: string;
	    message: string;
	
	    static createFrom(source: any = {}) {
	        return new BaiduLoginResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.success = source["success"];
	        this.username = source["username"];
	        this.user_id = source["user_id"];
	        this.bduss = source["bduss"];
	        this.ptoken = source["ptoken"];
	        this.stoken = source["stoken"];
	        this.bd_stoken = source["bd_stoken"];
	        this.vip_type = source["vip_type"];
	        this.photo_url = source["photo_url"];
	        this.message = source["message"];
	    }
	}
	export class BaiduPollResult {
	    status: string;
	    v: string;
	
	    static createFrom(source: any = {}) {
	        return new BaiduPollResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.status = source["status"];
	        this.v = source["v"];
	    }
	}
	export class BaiduQRCode {
	    qr_base64: string;
	    sign: string;
	    prompt: string;
	
	    static createFrom(source: any = {}) {
	        return new BaiduQRCode(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.qr_base64 = source["qr_base64"];
	        this.sign = source["sign"];
	        this.prompt = source["prompt"];
	    }
	}
	export class SystemInfo {
	    os: string;
	    arch: string;
	    num_cpu: number;
	    hostname: string;
	    go_ver: string;
	    time: string;
	    process_name: string;
	
	    static createFrom(source: any = {}) {
	        return new SystemInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.os = source["os"];
	        this.arch = source["arch"];
	        this.num_cpu = source["num_cpu"];
	        this.hostname = source["hostname"];
	        this.go_ver = source["go_ver"];
	        this.time = source["time"];
	        this.process_name = source["process_name"];
	    }
	}

}

