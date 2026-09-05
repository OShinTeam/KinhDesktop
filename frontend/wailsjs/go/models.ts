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
	
	export class AppSettings {
	    language: string;
	    close_action: string;
	    download_user_agent: string;
	    download_threads: number;
	    download_dir: string;
	    log_level: string;
	
	    static createFrom(source: any = {}) {
	        return new AppSettings(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.language = source["language"];
	        this.close_action = source["close_action"];
	        this.download_user_agent = source["download_user_agent"];
	        this.download_threads = source["download_threads"];
	        this.download_dir = source["download_dir"];
	        this.log_level = source["log_level"];
	    }
	}
	export class BaiduFileItem {
	    fs_id: number;
	    path: string;
	    filename: string;
	    server_filename: string;
	    size: number;
	    isdir: number;
	    server_mtime: number;
	    category: number;
	    md5?: string;
	
	    static createFrom(source: any = {}) {
	        return new BaiduFileItem(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.fs_id = source["fs_id"];
	        this.path = source["path"];
	        this.filename = source["filename"];
	        this.server_filename = source["server_filename"];
	        this.size = source["size"];
	        this.isdir = source["isdir"];
	        this.server_mtime = source["server_mtime"];
	        this.category = source["category"];
	        this.md5 = source["md5"];
	    }
	}
	export class BaiduFileListResult {
	    success: boolean;
	    dir: string;
	    list: BaiduFileItem[];
	    message?: string;
	
	    static createFrom(source: any = {}) {
	        return new BaiduFileListResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.success = source["success"];
	        this.dir = source["dir"];
	        this.list = this.convertValues(source["list"], BaiduFileItem);
	        this.message = source["message"];
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
	export class BaiduQuotaInfo {
	    success: boolean;
	    total: number;
	    used: number;
	    message?: string;
	
	    static createFrom(source: any = {}) {
	        return new BaiduQuotaInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.success = source["success"];
	        this.total = source["total"];
	        this.used = source["used"];
	        this.message = source["message"];
	    }
	}
	export class OShinDInfo {
	    installed: boolean;
	    version: string;
	    repo_url: string;
	
	    static createFrom(source: any = {}) {
	        return new OShinDInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.installed = source["installed"];
	        this.version = source["version"];
	        this.repo_url = source["repo_url"];
	    }
	}
	export class OShinDUpdateResult {
	    success: boolean;
	    message: string;
	    latest_version: string;
	    has_update: boolean;
	    changelog: string;
	    page_url: string;
	
	    static createFrom(source: any = {}) {
	        return new OShinDUpdateResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.success = source["success"];
	        this.message = source["message"];
	        this.latest_version = source["latest_version"];
	        this.has_update = source["has_update"];
	        this.changelog = source["changelog"];
	        this.page_url = source["page_url"];
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
	export class UpdateCheckResult {
	    success: boolean;
	    message: string;
	    current_version: string;
	    latest_version: string;
	    has_update: boolean;
	    changelog: string;
	    page_url: string;
	
	    static createFrom(source: any = {}) {
	        return new UpdateCheckResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.success = source["success"];
	        this.message = source["message"];
	        this.current_version = source["current_version"];
	        this.latest_version = source["latest_version"];
	        this.has_update = source["has_update"];
	        this.changelog = source["changelog"];
	        this.page_url = source["page_url"];
	    }
	}

}

